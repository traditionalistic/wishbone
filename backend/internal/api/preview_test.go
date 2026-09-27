package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const previewSrc = `{"id": "t.prev", "title": "Превью", "script": "data me = demo.profile()\nhandler go() { call demo.claimOffer({id: \"o1\"}) }", "timelines": [{"name": "intro", "tracks": [{"target": "root", "prop": "opacity", "keys": [{"t": 0, "v": 0}, {"t": 300, "v": 1}]}]}],
 "root": {"type": "stack", "id": "root", "on": {"tap": "go()"}, "children": []}}`

func (e *env) ws(path string) *websocket.Conn {
	e.t.Helper()
	c, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(e.url, "http")+path, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { c.CloseNow() })
	return c
}

func recv(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return obj(t, b)
}

func say(t *testing.T, c *websocket.Conn, msg string) {
	t.Helper()
	if err := c.Write(context.Background(), websocket.MessageText, []byte(msg)); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewSession(t *testing.T) {
	e := newEnv(t)
	code, b := e.admin("POST", "/preview", map[string]string{"screen": "t.prev"})
	session, _ := obj(t, b)["code"].(string)
	if code != 200 || len(session) != 4 {
		t.Fatalf("create: %d %s", code, b)
	}
	if code, b := e.admin("POST", "/preview", nil); code != 200 || obj(t, b)["code"] == session {
		t.Fatalf("a session without a screen: %d %s", code, b)
	}
	_, _, b = e.do("GET", "/v1/preview/sessions", nil)
	if !strings.Contains(string(b), `"code":"`+session+`","screen":"t.prev"`) {
		t.Fatalf("sessions: %s", b)
	}

	admin := e.ws("/admin/api/preview/" + session + "/ws?token=" + password)
	if m := recv(t, admin); m["type"] != "devices" || len(m["list"].([]any)) != 0 {
		t.Fatalf("admin hello: %v", m)
	}
	dev := e.ws("/v1/preview/ws?session=" + session)
	say(t, dev, `{"type":"hello","platform":"android","device":"Pixel 9","runtime":"1.0.0"}`)
	devs := recv(t, admin)
	if get(devs, "list", 0, "device") != "Pixel 9" || get(devs, "list", 0, "platform") != "android" {
		t.Fatalf("devices: %v", devs)
	}

	code, b = e.admin("POST", "/preview/"+session+"/document", map[string]any{"source": json.RawMessage(previewSrc)})
	if m := obj(t, b); code != 200 || m["ok"] != true || len(m["diags"].([]any)) != 0 {
		t.Fatalf("push: %d %s", code, b)
	}
	doc := recv(t, dev)
	if doc["type"] != "document" || doc["screen"] != "t.prev" || get(doc, "document", "revision") != 0 || get(doc, "document", "root", "id") != "root" {
		t.Fatalf("device document: %v", doc)
	}
	if get(doc, "data", "me", "value", "avatar") != e.url+"/assets/avatar.png" {
		t.Fatalf("asset URLs use the host the device connected through: %v", get(doc, "data", "me"))
	}

	late := e.ws("/v1/preview/ws?session=" + session)
	say(t, late, `{"type":"hello","platform":"ios","device":"iPhone","runtime":"1.0.0"}`)
	if m := recv(t, late); m["type"] != "document" || m["screen"] != "t.prev" {
		t.Fatalf("a joining device gets the last document: %v", m)
	}
	if m := recv(t, admin); len(m["list"].([]any)) != 2 {
		t.Fatalf("devices: %v", m)
	}

	bad := strings.Replace(previewSrc, `data me = demo.profile()`, `data me = demo.nope()`, 1)
	code, b = e.admin("POST", "/preview/"+session+"/document", map[string]any{"source": json.RawMessage(bad)})
	if m := obj(t, b); code != 200 || m["ok"] != false || get(m, "diags", 0, "path") != "script" {
		t.Fatalf("push with errors: %d %s", code, b)
	}

	if code, b := e.admin("POST", "/preview/"+session+"/play", map[string]string{"timeline": "intro"}); code != 204 {
		t.Fatalf("play: %d %s", code, b)
	}
	if m := recv(t, dev); m["type"] != "play" || m["timeline"] != "intro" {
		t.Fatalf("play (and no document for the failed push): %v", m)
	}

	say(t, dev, `{"type":"log","level":"error","message":"boom"}`)
	if m := recv(t, admin); m["type"] != "log" || m["level"] != "error" || m["message"] != "boom" || m["device"] != get(devs, "list", 0, "id") {
		t.Fatalf("log: %v", m)
	}

	// calls from a preview document (revision 0) are bound through the session
	call := func(op string, args any) (int, []byte) {
		code, _, b := e.do("POST", "/v1/call", map[string]any{"screen": "t.prev", "revision": 0, "op": op, "args": args, "idempotencyKey": "pk-" + op})
		return code, b
	}
	if code, b := call("demo.claimOffer", map[string]any{"id": "o1"}); code != 200 || get(obj(t, b), "value", "tickets") != 15 {
		t.Fatalf("preview call: %d %s", code, b)
	}
	code, b = call("demo.subscribe", map[string]any{"email": "a@b.co", "name": "I", "plan": "pro", "promo": ""})
	wantErr(t, code, b, 403, "OPERATION_NOT_BOUND")
	code, _, b = e.do("POST", "/v1/call", map[string]any{"screen": "t.nosession", "revision": 0, "op": "demo.claimOffer", "args": map[string]any{"id": "o1"}, "idempotencyKey": "x"})
	wantErr(t, code, b, 409, "REVISION_NOT_FOUND")
	code, _, b = e.do("POST", "/v1/data", map[string]any{"screen": "t.prev", "revision": 0, "sources": []string{"me"}})
	if code != 200 || get(obj(t, b), "me", "ok") != true {
		t.Fatalf("preview data: %d %s", code, b)
	}
}

func TestPreviewUnknownSession(t *testing.T) {
	e := newEnv(t)
	code, b := e.admin("POST", "/preview/ZZZZ/document", map[string]any{"source": json.RawMessage(previewSrc)})
	wantErr(t, code, b, 404, "NOT_FOUND")
	code, b = e.admin("POST", "/preview/ZZZZ/play", map[string]string{"timeline": "intro"})
	wantErr(t, code, b, 404, "NOT_FOUND")
	for _, p := range []string{"/v1/preview/ws?session=ZZZZ", "/admin/api/preview/ZZZZ/ws?token=" + password} {
		if _, resp, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(e.url, "http")+p, nil); err == nil || resp.StatusCode != 404 {
			t.Fatalf("%s: %v", p, err)
		}
	}
	if _, resp, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(e.url, "http")+"/admin/api/preview/ZZZZ/ws?token=nope", nil); err == nil || resp.StatusCode != 401 {
		t.Fatalf("admin ws without the password: %v", err)
	}
}
