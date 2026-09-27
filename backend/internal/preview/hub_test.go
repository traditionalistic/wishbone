package preview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
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

	if !h.SetDocument(code, "s.x", "Промо", json.RawMessage(doc), json.RawMessage(`{"ops":["demo.subscribe"]}`)) {
		t.Fatal("SetDocument on a live session")
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
	if h.SetDocument("ZZZZ", "s", "", json.RawMessage(doc), nil) || h.Play("ZZZZ", "x") {
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
