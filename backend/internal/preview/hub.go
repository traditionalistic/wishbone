// Package preview is the live-preview hub (contract/api.md, spec §5.4): in-memory sessions that fan the admin's
// compiled documents out to device WebSockets and device logs back to admin WebSockets. Render-capable devices also
// shoot the screens of the admin's board (admin preview spec §1): the hub hands each one job at a time and keeps the
// last shot of every (screen, device) for the admins.
package preview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"wishbone/backend/internal/compile"
)

const (
	alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	TTL      = 2 * time.Hour
	// Base stands for scheme://host inside document messages; each device gets the host it connected through
	// (localhost for the iOS simulator, 10.0.2.2 for the Android emulator).
	Base = "http://wishbone-base.invalid"

	JobTimeout = 15 * time.Second
	// coder/websocket reads 32 KiB by default: a device's first shot would drop the connection
	deviceReadLimit = 4 << 20
	maxShot         = 1 << 20 // decoded JPEG bytes
	maxShotSide     = 2000    // pt/dp
	maxShotError    = 200     // runes
)

type Hub struct {
	mu         sync.Mutex
	sessions   map[string]*session
	seq, jobs  int
	now        func() time.Time
	jobTimeout time.Duration
}

// Screen is one screen of a board: the focus compiled from the admin's source, or a stored draft linked to it.
type Screen struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Links    []compile.Link  `json:"links"`
	Error    string          `json:"error,omitempty"` // set = a ghost card, never rendered
	Document json.RawMessage `json:"-"`
	Data     json.RawMessage `json:"-"` // resolved with Base for the host
	Meta     json.RawMessage `json:"-"` // what /v1/call and /v1/data read for revision 0; used for the focus
}

// Board is the admin's open screen and the screens linked to it: Screens[0] is the focus, then BFS order.
type Board struct {
	Focus   string   `json:"focus"`
	Screens []Screen `json:"screens"`
}

type session struct {
	code, screen, title string
	msg                 []byte // last {"type":"document",…} message, with Base for the host
	meta                json.RawMessage
	board               Board
	boardMsg            []byte                       // last {"type":"board",…} message
	hash                map[string][sha256.Size]byte // content of each renderable board screen, by id
	frames              map[frameKey]*frame          // last shot of each (screen, device); ready or failed
	peers               map[*peer]bool
	updated             time.Time
}

type peer struct {
	c                    *websocket.Conn
	out                  chan []byte
	admin                bool
	base                 string
	id, platform, device string // devices, set by hello
	render               bool   // the device answers render jobs
	job                  *job   // the render job in flight; a device has at most one
}

type job struct {
	id     int
	screen string
	hash   [sha256.Size]byte
	timer  *time.Timer
}

type frameKey struct{ screen, device string }

// frame is the wire frame (contract/api.md); hash is the content it was shot for.
type frame struct {
	Screen  string `json:"screen"`
	Device  string `json:"device"`
	Status  string `json:"status"` // "rendering" | "ready" | "failed"
	Src     string `json:"src,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	Settled *bool  `json:"settled,omitempty"`
	Error   string `json:"error,omitempty"`
	hash    [sha256.Size]byte
}

type Summary struct {
	Code      string    `json:"code"`
	Screen    string    `json:"screen"`
	Title     string    `json:"title"`
	Devices   int       `json:"devices"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func New() *Hub {
	return &Hub{sessions: map[string]*session{}, now: time.Now, jobTimeout: JobTimeout}
}

func (h *Hub) Create(screen, title string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	for {
		b := make([]byte, 4)
		for i := range b {
			b[i] = alphabet[rand.IntN(len(alphabet))]
		}
		if code := string(b); h.sessions[code] == nil {
			h.sessions[code] = &session{code: code, screen: screen, title: title, frames: map[frameKey]*frame{},
				peers: map[*peer]bool{}, updated: h.now()}
			return code
		}
	}
}

func (h *Hub) live(s *session) bool { return h.now().Sub(s.updated) < TTL }

// get returns a live session; the caller holds h.mu.
func (h *Hub) get(code string) *session {
	if s := h.sessions[code]; s != nil && h.live(s) {
		return s
	}
	return nil
}

func (h *Hub) Exists(code string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.get(code) != nil
}

func (h *Hub) Sessions() []Summary {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []Summary{}
	for _, s := range h.sessions {
		if h.live(s) {
			out = append(out, Summary{Code: s.code, Screen: s.screen, Title: s.title, Devices: len(devices(s)), UpdatedAt: s.updated})
		}
	}
	slices.SortFunc(out, func(a, b Summary) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out
}

// Push shows a board in the session: the focus document goes to every device, the board to the admins, and
// render devices get jobs for the screens whose content has no shot yet. False = unknown session.
func (h *Hub) Push(code string, b Board) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.get(code)
	if s == nil {
		return false
	}
	focus := b.Screens[0]
	s.screen, s.meta, s.updated = focus.ID, focus.Meta, h.now()
	if focus.Title != "" {
		s.title = focus.Title
	}
	s.msg = mustJSON(struct {
		Type     string          `json:"type"`
		Screen   string          `json:"screen"`
		Document json.RawMessage `json:"document"`
		Data     json.RawMessage `json:"data"`
	}{"document", focus.ID, focus.Document, focus.Data})
	for _, p := range devices(s) {
		p.send(rebase(s.msg, p.base))
	}

	s.board, s.hash = b, map[string][sha256.Size]byte{}
	for _, sc := range b.Screens {
		if sc.Error == "" {
			s.hash[sc.ID] = contentHash(sc)
		}
	}
	for k := range s.frames {
		if _, ok := s.hash[k.screen]; !ok {
			delete(s.frames, k)
		}
	}
	if msg := mustJSON(struct {
		Type string `json:"type"`
		Board
	}{"board", b}); !bytes.Equal(msg, s.boardMsg) {
		s.boardMsg = msg
		broadcastAdmins(s, msg)
	}
	h.pump(s)
	return true
}

func contentHash(sc Screen) [sha256.Size]byte {
	d := sha256.New()
	d.Write(sc.Document)
	d.Write([]byte{0}) // JSON never holds a raw NUL
	d.Write(sc.Data)
	return [sha256.Size]byte(d.Sum(nil))
}

// Retry drops the session's failed frames, so their screens render again. False = unknown session.
func (h *Hub) Retry(code string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.get(code)
	if s == nil {
		return false
	}
	s.updated = h.now()
	for k, f := range s.frames {
		if f.Status == "failed" {
			delete(s.frames, k)
		}
	}
	h.pump(s)
	return true
}

// pump hands every idle render device its next job. Jobs go out only while an admin watches the session.
func (h *Hub) pump(s *session) {
	watched := false
	for p := range s.peers {
		watched = watched || p.admin
	}
	if !watched {
		return
	}
	for _, p := range devices(s) {
		if !p.render || p.job != nil {
			continue
		}
		sc := next(s, p)
		if sc == nil {
			continue
		}
		h.jobs++
		j := &job{id: h.jobs, screen: sc.ID, hash: s.hash[sc.ID]}
		j.timer = time.AfterFunc(h.jobTimeout, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if p.job == j {
				h.finish(s, p, &frame{Status: "failed", Error: "timeout"})
			}
		})
		p.job = j
		p.send(rebase(mustJSON(struct {
			Type     string          `json:"type"`
			Job      int             `json:"job"`
			Screen   string          `json:"screen"`
			Document json.RawMessage `json:"document"`
			Data     json.RawMessage `json:"data"`
		}{"render", j.id, sc.ID, sc.Document, sc.Data}), p.base))
		broadcastAdmins(s, frameMsg(&frame{Screen: sc.ID, Device: p.id, Status: "rendering"}))
	}
}

// next is the device's next job: the first renderable board screen without a frame of this device for its current
// content. A failed frame counts as done until Retry or new content.
func next(s *session, p *peer) *Screen {
	for i := range s.board.Screens {
		sc := &s.board.Screens[i]
		if sc.Error != "" {
			continue
		}
		if f := s.frames[frameKey{sc.ID, p.id}]; f == nil || f.hash != s.hash[sc.ID] {
			return sc
		}
	}
	return nil
}

// finish ends the device's job with f and hands the device its next one. A shot of content that has changed since
// is still kept (a new picture beats none); next then asks for the current content.
func (h *Hub) finish(s *session, p *peer, f *frame) {
	j := p.job
	j.timer.Stop()
	p.job = nil
	if _, ok := s.hash[j.screen]; ok {
		f.Screen, f.Device, f.hash = j.screen, p.id, j.hash
		s.frames[frameKey{j.screen, p.id}] = f
		broadcastAdmins(s, frameMsg(f))
	}
	h.pump(s)
}

func (h *Hub) Play(code, timeline string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.get(code)
	if s == nil {
		return false
	}
	s.updated = h.now()
	msg := mustJSON(struct {
		Type     string `json:"type"`
		Timeline string `json:"timeline"`
	}{"play", timeline})
	for _, p := range devices(s) {
		p.send(msg)
	}
	return true
}

// Meta is the meta of the newest live session showing screen: preview documents are revision 0, and
// /v1/call and /v1/data resolve revision 0 through it.
func (h *Hub) Meta(screen string) (json.RawMessage, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var best *session
	for _, s := range h.sessions {
		if s.screen == screen && s.meta != nil && h.live(s) && (best == nil || s.updated.After(best.updated)) {
			best = s
		}
	}
	if best == nil {
		return nil, false
	}
	return best.meta, true
}

// Sweep drops sessions idle for TTL and closes their sockets.
func (h *Hub) Sweep() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for code, s := range h.sessions {
		if !h.live(s) {
			delete(h.sessions, code)
			closeAll(s, "session expired")
		}
	}
}

// Run sweeps every minute until ctx ends, then closes every socket.
func (h *Hub) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			h.Sweep()
		case <-ctx.Done():
			h.mu.Lock()
			for _, s := range h.sessions {
				closeAll(s, "server shutdown")
			}
			h.mu.Unlock()
			return
		}
	}
}

func closeAll(s *session, reason string) {
	for p := range s.peers {
		go p.c.Close(websocket.StatusGoingAway, reason)
	}
}

// ServeDevice runs a device socket; base is the device's scheme://host. False (nothing written) = unknown session.
func (h *Hub) ServeDevice(w http.ResponseWriter, r *http.Request, code, base string) bool {
	return h.serve(w, r, code, &peer{base: base})
}

// ServeAdmin runs an admin socket (devices, board, frames and device logs). False (nothing written) = unknown session.
func (h *Hub) ServeAdmin(w http.ResponseWriter, r *http.Request, code string) bool {
	return h.serve(w, r, code, &peer{admin: true})
}

func (h *Hub) serve(w http.ResponseWriter, r *http.Request, code string, p *peer) bool {
	if !h.Exists(code) {
		return false
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return true // Accept wrote the response
	}
	defer c.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	p.c, p.out = c, make(chan []byte, 32)
	go p.writeLoop(ctx)

	h.mu.Lock()
	s := h.get(code)
	if s == nil {
		h.mu.Unlock()
		c.Close(websocket.StatusGoingAway, "session expired")
		return true
	}
	s.peers[p] = true
	s.updated = h.now()
	if p.admin {
		p.send(devicesMsg(s))
		if s.boardMsg != nil {
			// one message for every frame: the peer queue holds 32, a 16-screen board on two devices would overflow it
			p.send(s.boardMsg)
			p.send(framesMsg(s))
		}
		h.pump(s)
	}
	h.mu.Unlock()
	defer h.leave(s, p)

	if p.admin {
		<-c.CloseRead(ctx).Done()
		return true
	}
	c.SetReadLimit(deviceReadLimit)
	for {
		_, b, err := c.Read(ctx)
		if err != nil {
			return true
		}
		h.onDevice(s, p, b)
	}
}

func (h *Hub) leave(s *session, p *peer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(s.peers, p)
	if p.job != nil {
		p.job.timer.Stop()
		p.job = nil
	}
	for k := range s.frames {
		if k.device == p.id {
			delete(s.frames, k)
		}
	}
	if p.id != "" {
		broadcastAdmins(s, devicesMsg(s))
	}
}

// deviceMsg is any device → server message (contract/api.md).
type deviceMsg struct {
	Type, Platform, Device, Level, Message string
	Render                                 bool
	Job                                    int
	Image, Error                           string
	Width, Height                          float64
	Settled                                bool
}

func (h *Hub) onDevice(s *session, p *peer, b []byte) {
	var m deviceMsg
	if json.Unmarshal(b, &m) != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s.updated = h.now()
	switch m.Type {
	case "hello":
		if p.id == "" {
			h.seq++
			p.id = fmt.Sprintf("d%d", h.seq)
		}
		p.platform, p.device, p.render = m.Platform, m.Device, m.Render
		if s.msg != nil {
			p.send(rebase(s.msg, p.base))
		}
		broadcastAdmins(s, devicesMsg(s))
		h.pump(s)
	case "log":
		if m.Level != "warn" && m.Level != "error" {
			m.Level = "info"
		}
		broadcastAdmins(s, mustJSON(struct {
			Type    string `json:"type"`
			Device  string `json:"device"`
			Level   string `json:"level"`
			Message string `json:"message"`
		}{"log", p.id, m.Level, m.Message}))
	case "shot":
		if p.job == nil || p.job.id != m.Job {
			return // stale: the job timed out (the device may have another since) or was never sent
		}
		h.finish(s, p, m.frame())
	}
}

// frame checks a shot at this unauthenticated boundary (the socket carries only the session code): a JPEG of at most
// maxShot bytes, 1…maxShotSide on each side. Anything else is "invalid shot".
func (m *deviceMsg) frame() *frame {
	if m.Error != "" {
		if r := []rune(m.Error); len(r) > maxShotError {
			m.Error = string(r[:maxShotError])
		}
		return &frame{Status: "failed", Error: m.Error}
	}
	img, err := base64.StdEncoding.DecodeString(m.Image)
	if err != nil || len(img) > maxShot || !bytes.HasPrefix(img, []byte{0xFF, 0xD8, 0xFF}) ||
		m.Width < 1 || m.Width > maxShotSide || m.Height < 1 || m.Height > maxShotSide {
		return &frame{Status: "failed", Error: "invalid shot"}
	}
	settled := m.Settled
	return &frame{Status: "ready", Src: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(img),
		Width: int(math.Round(m.Width)), Height: int(math.Round(m.Height)), Settled: &settled}
}

// devices lists the session's devices that said hello, by id.
func devices(s *session) []*peer {
	var out []*peer
	for p := range s.peers {
		if p.id != "" {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b *peer) int { return strings.Compare(a.id, b.id) })
	return out
}

type device struct {
	ID       string `json:"id"`
	Platform string `json:"platform"`
	Device   string `json:"device"`
	Render   bool   `json:"render"`
}

func devicesMsg(s *session) []byte {
	list := []device{}
	for _, p := range devices(s) {
		list = append(list, device{p.id, p.platform, p.device, p.render})
	}
	return mustJSON(struct {
		Type string   `json:"type"`
		List []device `json:"list"`
	}{"devices", list})
}

func frameMsg(f *frame) []byte {
	return mustJSON(struct {
		Type string `json:"type"`
		*frame
	}{"frame", f})
}

// framesMsg lists the stored frames by board screen and device, each device's job in flight as "rendering" after its
// stored frame (the admin keeps the old picture under the spinner).
func framesMsg(s *session) []byte {
	list := []*frame{}
	for _, sc := range s.board.Screens {
		for _, p := range devices(s) {
			if f := s.frames[frameKey{sc.ID, p.id}]; f != nil {
				list = append(list, f)
			}
			if p.job != nil && p.job.screen == sc.ID {
				list = append(list, &frame{Screen: sc.ID, Device: p.id, Status: "rendering"})
			}
		}
	}
	return mustJSON(struct {
		Type string   `json:"type"`
		List []*frame `json:"list"`
	}{"frames", list})
}

func broadcastAdmins(s *session, msg []byte) {
	for p := range s.peers {
		if p.admin {
			p.send(msg)
		}
	}
}

func rebase(msg []byte, base string) []byte {
	return bytes.ReplaceAll(msg, []byte(Base), []byte(base)) // one copy; was string(msg) + ReplaceAll + []byte(...)
}

// send queues without blocking; a peer that cannot keep up is disconnected.
func (p *peer) send(b []byte) {
	select {
	case p.out <- b:
	default:
		go p.c.Close(websocket.StatusPolicyViolation, "too slow")
	}
}

func (p *peer) writeLoop(ctx context.Context) {
	for {
		select {
		case b := <-p.out:
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := p.c.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// mustJSON marshals without HTML escaping: documents keep the exact bytes the admin compiled.
func mustJSON(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}
