package preview

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"wishbone/backend/internal/compile"
)

func server(t *testing.T, h *Hub) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/device", func(w http.ResponseWriter, r *http.Request) {
		if !h.ServeDevice(w, r, r.URL.Query().Get("code"), "http://dev.example:8080") {
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		if !h.ServeAdmin(w, r, r.URL.Query().Get("code")) {
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.SetReadLimit(-1) // admins get whole pictures, as a browser does
	t.Cleanup(func() { c.CloseNow() })
	return c
}

func send(t *testing.T, c *websocket.Conn, msg string) {
	t.Helper()
	if err := c.Write(context.Background(), websocket.MessageText, []byte(msg)); err != nil {
		t.Fatal(err)
	}
}

func recv(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const doc = `{"type":"document","screen":"s.x","document":{"root":1},"data":{"me":{"ok":true,"value":{"avatar":"` + Base + `/assets/a.png"}}}}`

var focusX = Screen{ID: "s.x", Title: "Промо", Links: []compile.Link{}, Document: json.RawMessage(`{"root":1}`),
	Data: json.RawMessage(`{"me":{"ok":true,"value":{"avatar":"` + Base + `/assets/a.png"}}}`), Meta: json.RawMessage(`{"ops":["demo.subscribe"]}`)}

func TestDeviceReceivesDocuments(t *testing.T) {
	h := New()
	url := server(t, h)
	code := h.Create("s.x", "Экран")
	if len(code) != 4 || strings.Trim(code, alphabet) != "" {
		t.Fatalf("code %q", code)
	}
	d1 := dial(t, url+"/device?code="+code)
	send(t, d1, `{"type":"hello","platform":"ios","device":"iPhone 17 Pro","runtime":"1.0.0"}`)
	waitDevices(t, h, code, 1)

	if !h.Push(code, Board{Focus: "s.x", Screens: []Screen{focusX}}) {
		t.Fatal("Push on a live session")
	}
	want := strings.ReplaceAll(doc, Base, "http://dev.example:8080")
	if got := recv(t, d1); got != want {
		t.Fatalf("device got %s\nwant %s", got, want)
	}

	d2 := dial(t, url+"/device?code="+code)
	send(t, d2, `{"type":"hello","platform":"android","device":"Pixel","runtime":"1.0.0"}`)
	if got := recv(t, d2); got != want {
		t.Fatalf("a joining device gets the last document: %s", got)
	}

	if !h.Play(code, "intro") {
		t.Fatal("play")
	}
	for _, c := range []*websocket.Conn{d1, d2} {
		if got := recv(t, c); got != `{"type":"play","timeline":"intro"}` {
			t.Fatalf("play: %s", got)
		}
	}

	s := h.Sessions()
	if len(s) != 1 || s[0].Code != code || s[0].Screen != "s.x" || s[0].Title != "Промо" || s[0].Devices != 2 {
		t.Fatalf("sessions %+v", s)
	}
	if m, ok := h.Meta("s.x"); !ok || string(m) != `{"ops":["demo.subscribe"]}` {
		t.Fatalf("meta %s %v", m, ok)
	}
	if _, ok := h.Meta("s.other"); ok {
		t.Fatal("meta of a screen without a session")
	}
}

func waitDevices(t *testing.T, h *Hub, code string, n int) {
	t.Helper()
	for range 200 {
		for _, s := range h.Sessions() {
			if s.Code == code && s.Devices == n {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("session %s never reached %d devices: %+v", code, n, h.Sessions())
}

func TestAdminSeesDevicesAndLogs(t *testing.T) {
	h := New()
	url := server(t, h)
	code := h.Create("", "")
	a := dial(t, url+"/admin?code="+code)
	if got := recv(t, a); got != `{"type":"devices","list":[]}` {
		t.Fatalf("admin first message: %s", got)
	}
	d := dial(t, url+"/device?code="+code)
	send(t, d, `{"type":"hello","platform":"ios","device":"iPhone","runtime":"1.0.0"}`)
	var devs struct {
		Type string
		List []struct{ ID, Platform, Device string }
	}
	if err := json.Unmarshal([]byte(recv(t, a)), &devs); err != nil || devs.Type != "devices" || len(devs.List) != 1 ||
		devs.List[0].Platform != "ios" || devs.List[0].Device != "iPhone" || devs.List[0].ID == "" {
		t.Fatalf("devices %+v %v", devs, err)
	}
	send(t, d, `{"type":"log","level":"warn","message":"layout overflow"}`)
	want := `{"type":"log","device":"` + devs.List[0].ID + `","level":"warn","message":"layout overflow"}`
	if got := recv(t, a); got != want {
		t.Fatalf("log %s, want %s", got, want)
	}
	d.Close(websocket.StatusNormalClosure, "")
	if got := recv(t, a); got != `{"type":"devices","list":[]}` {
		t.Fatalf("after disconnect: %s", got)
	}
}

func TestUnknownAndExpiredSessions(t *testing.T) {
	h := New()
	var skew atomic.Int64 // the fake clock is read by server goroutines
	h.now = func() time.Time { return time.Now().Add(time.Duration(skew.Load())) }
	url := server(t, h)
	if _, resp, err := websocket.Dial(context.Background(), url+"/device?code=ZZZZ", nil); err == nil || resp.StatusCode != 404 {
		t.Fatalf("unknown session must 404: %v", err)
	}
	if h.Push("ZZZZ", Board{Focus: "s.x", Screens: []Screen{focusX}}) || h.Play("ZZZZ", "x") || h.Retry("ZZZZ") {
		t.Fatal("unknown session accepted")
	}
	code := h.Create("s.y", "")
	d := dial(t, url+"/device?code="+code)
	skew.Store(int64(TTL - time.Minute))
	h.Sweep()
	if !h.Exists(code) {
		t.Fatal("expired too early")
	}
	skew.Store(int64(TTL + time.Minute))
	h.Sweep()
	if h.Exists(code) || len(h.Sessions()) != 0 {
		t.Fatal("idle session must expire after TTL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, _, err := d.Read(ctx); websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Fatalf("sockets of an expired session are closed: %v", err)
	}
}

// --- render jobs ---

func obj(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	var m map[string]any
	if s := recv(t, c); json.Unmarshal([]byte(s), &m) != nil {
		t.Fatalf("not a JSON object: %s", s)
	}
	return m
}

func screen(id, document string) Screen {
	return Screen{ID: id, Title: id, Links: []compile.Link{}, Document: json.RawMessage(document), Data: json.RawMessage(`{}`)}
}

// jpeg is n bytes that start like a JPEG, base64-encoded.
func jpeg(n int) string {
	return base64.StdEncoding.EncodeToString(append([]byte{0xFF, 0xD8, 0xFF}, bytes.Repeat([]byte{7}, n-3)...))
}

// shot answers a job with a 600 KiB picture: far past coder/websocket's default 32 KiB read limit.
func shot(t *testing.T, dev *websocket.Conn, job any) {
	t.Helper()
	send(t, dev, fmt.Sprintf(`{"type":"shot","job":%v,"image":"%s","width":402,"height":874,"settled":true}`, job, jpeg(600<<10)))
}

// renderJob reads the device's next message, which must be a render job of screen; it returns the job id.
func renderJob(t *testing.T, dev *websocket.Conn, screen string) any {
	t.Helper()
	m := obj(t, dev)
	if m["type"] != "render" || m["screen"] != screen || m["job"] == nil {
		t.Fatalf("want a render job of %s, got %.300v", screen, m)
	}
	return m["job"]
}

func wantFrame(t *testing.T, admin *websocket.Conn, screen, status string) map[string]any {
	t.Helper()
	m := obj(t, admin)
	if m["type"] != "frame" || m["screen"] != screen || m["status"] != status {
		t.Fatalf("want frame %s %s, got %.300v", screen, status, m)
	}
	return m
}

// expect reads c's next message, which must contain marker. Messages to a peer keep their order, so a marker after an
// action proves nothing else was queued in between.
func expect(t *testing.T, c *websocket.Conn, marker string) {
	t.Helper()
	if got := recv(t, c); !strings.Contains(got, marker) {
		t.Fatalf("want %s next, got %.300s", marker, got)
	}
}

// watchedSession is a session with an admin and a render device, both past their greetings.
func watchedSession(t *testing.T, h *Hub, url string) (code string, admin, dev *websocket.Conn) {
	t.Helper()
	code = h.Create("", "")
	admin = dial(t, url+"/admin?code="+code)
	recv(t, admin)
	dev = dial(t, url+"/device?code="+code)
	send(t, dev, `{"type":"hello","platform":"ios","device":"iPhone 17 Pro","runtime":"1.1.0","render":true}`)
	if got := recv(t, admin); !strings.Contains(got, `{"id":"d1","platform":"ios","device":"iPhone 17 Pro","render":true}`) {
		t.Fatalf("devices: %s", got)
	}
	return code, admin, dev
}

func TestPushRendersBoard(t *testing.T) {
	h := New()
	url := server(t, h)
	code, admin, dev := watchedSession(t, h, url)
	old := dial(t, url+"/device?code="+code)
	send(t, old, `{"type":"hello","platform":"android","device":"Pixel","runtime":"1.0.0"}`)
	expect(t, admin, `"render":false`)

	b := Board{Focus: "s.a", Screens: []Screen{screen("s.a", `{"a":1}`), screen("s.b", `{"img":"`+Base+`/b.png"}`),
		{ID: "s.gone", Links: []compile.Link{}, Error: "screen not found"}}}
	h.Push(code, b)
	if got := recv(t, admin); got != `{"type":"board","focus":"s.a","screens":[{"id":"s.a","title":"s.a","links":[]},`+
		`{"id":"s.b","title":"s.b","links":[]},{"id":"s.gone","title":"","links":[],"error":"screen not found"}]}` {
		t.Fatalf("board: %s", got)
	}
	expect(t, dev, `{"type":"document","screen":"s.a"`)
	job := renderJob(t, dev, "s.a")
	wantFrame(t, admin, "s.a", "rendering")

	shot(t, dev, job)
	f := wantFrame(t, admin, "s.a", "ready")
	if f["device"] != "d1" || !strings.HasPrefix(f["src"].(string), "data:image/jpeg;base64,/9j/") ||
		f["width"] != 402.0 || f["height"] != 874.0 || f["settled"] != true {
		t.Fatalf("ready frame: %.200v", f)
	}
	m := obj(t, dev)
	if m["type"] != "render" || m["screen"] != "s.b" || get(m, "document", "img") != "http://dev.example:8080/b.png" {
		t.Fatalf("the next job is s.b, with the device's host: %v", m)
	}
	wantFrame(t, admin, "s.b", "rendering")
	shot(t, dev, m["job"])
	wantFrame(t, admin, "s.b", "ready")

	h.Play(code, "intro")
	expect(t, dev, `"play"`) // the ghost is never rendered
	expect(t, old, `"document"`)
	expect(t, old, `"play"`) // a playground without render gets the preview, never a job

	h.Push(code, b)
	expect(t, dev, `"document"`)
	h.Play(code, "intro")
	expect(t, dev, `"play"`) // the same board again: no job…
	send(t, dev, `{"type":"log","message":"mark"}`)
	expect(t, admin, `"mark"`) // …and no board or frame for the admin

	b.Screens[1] = screen("s.b", `{"b":2}`)
	h.Push(code, b)
	expect(t, dev, `"document"`)
	renderJob(t, dev, "s.b") // only the changed screen renders again
	wantFrame(t, admin, "s.b", "rendering")
}

func get(v any, path ...string) any {
	for _, k := range path {
		m, _ := v.(map[string]any)
		v = m[k]
	}
	return v
}

func TestStaleAndFailedShots(t *testing.T) {
	h := New()
	h.jobTimeout = 100 * time.Millisecond
	code, admin, dev := watchedSession(t, h, server(t, h))
	h.Push(code, Board{Focus: "s.a", Screens: []Screen{screen("s.a", `{"a":1}`)}})
	expect(t, admin, `"board"`)
	expect(t, dev, `"document"`)
	job := renderJob(t, dev, "s.a")
	wantFrame(t, admin, "s.a", "rendering")

	send(t, dev, fmt.Sprintf(`{"type":"shot","job":%v,"image":"%s","width":1,"height":1}`, job.(float64)+1, jpeg(10)))
	send(t, dev, `{"type":"log","message":"mark"}`)
	expect(t, admin, `"mark"`) // an answer to a job never sent is dropped

	if f := wantFrame(t, admin, "s.a", "failed"); f["error"] != "timeout" {
		t.Fatalf("an unanswered job fails: %v", f)
	}
	shot(t, dev, job)
	send(t, dev, `{"type":"log","message":"mark"}`)
	expect(t, admin, `"mark"`) // the late answer is dropped
	h.Play(code, "intro")
	expect(t, dev, `"play"`) // a failed frame counts as done: no new job

	if !h.Retry(code) {
		t.Fatal("retry on a live session")
	}
	retried := renderJob(t, dev, "s.a")
	if retried == job {
		t.Fatalf("a retry is a new job: %v", retried)
	}
	wantFrame(t, admin, "s.a", "rendering")
	send(t, dev, fmt.Sprintf(`{"type":"shot","job":%v,"image":"%s","width":402,"height":874}`, retried,
		base64.StdEncoding.EncodeToString([]byte("GIF89a not a jpeg"))))
	if f := wantFrame(t, admin, "s.a", "failed"); f["error"] != "invalid shot" || f["src"] != nil {
		t.Fatalf("a shot that is not a JPEG fails: %v", f)
	}
}

// Jobs go out only while an admin watches; a joining admin gets every stored frame in one message; a device that
// leaves takes its frames with it.
func TestAdminJoinAndDeviceLeave(t *testing.T) {
	h := New()
	url := server(t, h)
	code := h.Create("", "")
	dev := dial(t, url+"/device?code="+code)
	send(t, dev, `{"type":"hello","platform":"android","device":"Pixel 9","runtime":"1.1.0","render":true}`)
	waitDevices(t, h, code, 1)
	h.Push(code, Board{Focus: "s.a", Screens: []Screen{screen("s.a", `{"a":1}`), screen("s.b", `{"b":1}`)}})
	expect(t, dev, `"document"`)
	h.Play(code, "intro")
	expect(t, dev, `"play"`) // nobody watches: no job

	a1 := dial(t, url+"/admin?code="+code)
	expect(t, a1, `"devices"`)
	expect(t, a1, `"board"`)
	expect(t, a1, `{"type":"frames","list":[]}`)
	wantFrame(t, a1, "s.a", "rendering") // the admin's arrival starts the jobs
	shot(t, dev, renderJob(t, dev, "s.a"))
	wantFrame(t, a1, "s.a", "ready")
	renderJob(t, dev, "s.b")
	wantFrame(t, a1, "s.b", "rendering")

	a2 := dial(t, url+"/admin?code="+code)
	expect(t, a2, `"devices"`)
	expect(t, a2, `"board"`)
	var frames struct {
		Type string
		List []map[string]any
	}
	if err := json.Unmarshal([]byte(recv(t, a2)), &frames); err != nil || frames.Type != "frames" || len(frames.List) != 2 ||
		frames.List[0]["screen"] != "s.a" || frames.List[0]["status"] != "ready" || frames.List[0]["src"] == nil ||
		frames.List[1]["screen"] != "s.b" || frames.List[1]["status"] != "rendering" {
		t.Fatalf("frames on join: %.300v %v", frames, err)
	}

	dev.Close(websocket.StatusNormalClosure, "")
	expect(t, a2, `{"type":"devices","list":[]}`)
	h.mu.Lock()
	kept := len(h.sessions[code].frames) // pictures of a gone device are never listed, so only memory would show them
	h.mu.Unlock()
	if kept != 0 {
		t.Fatalf("a device that leaves keeps %d frames", kept)
	}
	a3 := dial(t, url+"/admin?code="+code)
	expect(t, a3, `"devices"`)
	expect(t, a3, `"board"`)
	expect(t, a3, `{"type":"frames","list":[]}`)
}

func TestShotValidation(t *testing.T) {
	long := strings.Repeat("я", 250)
	tests := []struct {
		name string
		msg  deviceMsg
		want frame
	}{
		{"jpeg", deviceMsg{Image: jpeg(8), Width: 402, Height: 873.6, Settled: true},
			frame{Status: "ready", Src: "data:image/jpeg;base64," + jpeg(8), Width: 402, Height: 874}},
		{"largest", deviceMsg{Image: jpeg(maxShot), Width: maxShotSide, Height: 1}, frame{Status: "ready", Src: "data:image/jpeg;base64," + jpeg(maxShot), Width: maxShotSide, Height: 1}},
		{"too big", deviceMsg{Image: jpeg(maxShot + 1), Width: 402, Height: 874}, frame{Status: "failed", Error: "invalid shot"}},
		{"not a jpeg", deviceMsg{Image: base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G'}), Width: 402, Height: 874}, frame{Status: "failed", Error: "invalid shot"}},
		{"not base64", deviceMsg{Image: "/9j/?", Width: 402, Height: 874}, frame{Status: "failed", Error: "invalid shot"}},
		{"no size", deviceMsg{Image: jpeg(8)}, frame{Status: "failed", Error: "invalid shot"}},
		{"too wide", deviceMsg{Image: jpeg(8), Width: maxShotSide + 1, Height: 874}, frame{Status: "failed", Error: "invalid shot"}},
		{"too tall", deviceMsg{Image: jpeg(8), Width: 402, Height: maxShotSide + 1}, frame{Status: "failed", Error: "invalid shot"}},
		{"device error", deviceMsg{Error: "app is in the background"}, frame{Status: "failed", Error: "app is in the background"}},
		{"long error", deviceMsg{Error: long}, frame{Status: "failed", Error: long[:maxShotError*len("я")]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.msg.frame()
			settled := got.Settled != nil && *got.Settled
			got.Settled = nil
			if *got != tt.want || settled != tt.msg.Settled {
				t.Errorf("got %.120v (settled %v), want %.120v", *got, settled, tt.want)
			}
		})
	}
}
