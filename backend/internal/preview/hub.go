// Package preview is the live-preview hub (contract/api.md, spec §5.4): in-memory sessions that fan the admin's
// compiled documents out to device WebSockets and device logs back to admin WebSockets.
package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	TTL      = 2 * time.Hour
	// Base stands for scheme://host inside document messages; each device gets the host it connected through
	// (localhost for the iOS simulator, 10.0.2.2 for the Android emulator).
	Base = "http://wishbone-base.invalid"
)

type Hub struct {
	mu       sync.Mutex
	sessions map[string]*session
	seq      int
	now      func() time.Time
}

type session struct {
	code, screen, title string
	msg                 []byte // last {"type":"document",…} message, with Base for the host
	meta                json.RawMessage
	peers               map[*peer]bool
	updated             time.Time
}

type peer struct {
	c                    *websocket.Conn
	out                  chan []byte
	admin                bool
	base                 string
	id, platform, device string // devices, set by hello
}

type Summary struct {
	Code      string    `json:"code"`
	Screen    string    `json:"screen"`
	Title     string    `json:"title"`
	Devices   int       `json:"devices"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func New() *Hub { return &Hub{sessions: map[string]*session{}, now: time.Now} }

func (h *Hub) Create(screen, title string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	for {
		b := make([]byte, 4)
		for i := range b {
			b[i] = alphabet[rand.IntN(len(alphabet))]
		}
		if code := string(b); h.sessions[code] == nil {
			h.sessions[code] = &session{code: code, screen: screen, title: title, peers: map[*peer]bool{}, updated: h.now()}
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

// SetDocument stores the session's document message and sends it to every device; false = unknown session.
func (h *Hub) SetDocument(code, screen, title string, msg, meta json.RawMessage) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.get(code)
	if s == nil {
		return false
	}
	if screen != "" {
		s.screen = screen
	}
	if title != "" {
		s.title = title
	}
	s.msg, s.meta, s.updated = msg, meta, h.now()
	for _, p := range devices(s) {
		p.send(rebase(msg, p.base))
	}
	return true
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

// ServeAdmin runs an admin socket (devices list and device logs). False (nothing written) = unknown session.
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
	}
	h.mu.Unlock()
	defer h.leave(s, p)

	if p.admin {
		<-c.CloseRead(ctx).Done()
		return true
	}
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
	if p.id != "" {
		broadcastAdmins(s, devicesMsg(s))
	}
}

func (h *Hub) onDevice(s *session, p *peer, b []byte) {
	var m struct{ Type, Platform, Device, Level, Message string }
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
		p.platform, p.device = m.Platform, m.Device
		if s.msg != nil {
			p.send(rebase(s.msg, p.base))
		}
		broadcastAdmins(s, devicesMsg(s))
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
	}
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
}

func devicesMsg(s *session) []byte {
	list := []device{}
	for _, p := range devices(s) {
		list = append(list, device{p.id, p.platform, p.device})
	}
	return mustJSON(struct {
		Type string   `json:"type"`
		List []device `json:"list"`
	}{"devices", list})
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

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
