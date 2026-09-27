package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"wishbone/backend/internal/ops"
)

const promoScreen = `{"id": "t.promo", "title": "Promo",
 "script": "param code: string = \"WB10\"\ndata me = demo.profile()\ndata promo = demo.checkPromo({code: code})\nstate msg = \"\"\nhandler check() {\n  let r = await call demo.checkPromo({code: \"WB10\"})\n  msg = r.value.message ?? \"\"\n}\nhandler claim() { call demo.claimOffer({id: \"o1\"}) }\nhandler sub() { call demo.subscribe({email: \"a@b.co\", name: \"I\", plan: \"pro\", promo: \"\"}) }",
 "timelines": [],
 "root": {"type": "stack", "id": "root", "children": [{"type": "text", "id": "t", "props": {"text": "=me.name ?? \"\""}, "on": {"tap": "check()\nclaim()\nsub()"}}]}}`

func TestScreenFetch(t *testing.T) {
	e := newEnv(t)
	rev := e.publish(promoScreen)

	code, _, b := e.do("GET", "/v1/screens/t.promo", nil)
	if code != 200 {
		t.Fatalf("%d %s", code, b)
	}
	m := obj(t, b)
	etag, _ := m["etag"].(string)
	if !regexp.MustCompile(`^\d+-[0-9a-f]{16}$`).MatchString(etag) || m["revision"] != rev {
		t.Fatalf("etag %q revision %v (published %d)", etag, m["revision"], rev)
	}
	if get(m, "document", "screen") != "t.promo" || get(m, "document", "revision") != rev {
		t.Fatalf("document %v", m["document"])
	}
	if get(m, "data", "me", "ok") != true || get(m, "data", "me", "value", "name") != "Ivan" ||
		get(m, "data", "me", "value", "avatar") != e.url+"/assets/avatar.png" {
		t.Fatalf("data.me %v", get(m, "data", "me"))
	}
	if get(m, "data", "promo", "value", "discount") != 10 {
		t.Fatalf("param default must feed data args: %v", get(m, "data", "promo"))
	}

	_, _, b = e.do("GET", "/v1/screens/t.promo?params="+url.QueryEscape(`{"code":"WB25"}`), nil)
	if get(obj(t, b), "data", "promo", "value", "discount") != 25 {
		t.Fatalf("params override defaults: %s", b)
	}

	code, _, b = e.do("GET", "/v1/screens/t.promo", nil, "If-None-Match", etag)
	m = obj(t, b)
	if code != 200 || m["document"] != nil || m["etag"] != etag || get(m, "data", "me", "ok") != true {
		t.Fatalf("If-None-Match: %d %s", code, b)
	}
	_, _, b = e.do("GET", "/v1/screens/t.promo", nil, "If-None-Match", `"`+etag+`"`)
	if obj(t, b)["document"] != nil {
		t.Fatal("a quoted If-None-Match matches too")
	}

	_, _, b = e.do("GET", "/v1/screens/t.promo", nil, "X-Forwarded-Proto", "https")
	if get(obj(t, b), "data", "me", "value", "avatar") != "https://"+e.url[len("http://"):]+"/assets/avatar.png" {
		t.Fatalf("X-Forwarded-Proto: %s", b)
	}

	code, _, b = e.do("GET", "/v1/screens/t.none", nil)
	wantErr(t, code, b, 404, "SCREEN_NOT_FOUND")
	if get(obj(t, b), "error", "retryable") != false {
		t.Fatalf("retryable from errors.json: %s", b)
	}
	code, _, b = e.do("GET", "/v1/screens/t.promo?params=nope", nil)
	wantErr(t, code, b, 400, "ARGS_INVALID")
}

func TestUnsupportedRuntime(t *testing.T) {
	e := newEnv(t)
	e.publish(promoScreen)
	for runtime, want := range map[string]int{"1.0.0": 200, "": 200, "1.2.0": 200, "2.0.0-beta": 200, "1.0.0+build.5": 200, "1.1.0-rc.1+b7": 200, "0.9.9": 409, "0.9.0+b": 409, "junk": 409} {
		code, _, b := e.do("GET", "/v1/screens/t.promo", nil, "X-Wishbone-Runtime", runtime)
		if code != want {
			t.Errorf("runtime %q: %d %s", runtime, code, b)
		}
		if want == 409 {
			wantErr(t, code, b, 409, "UNSUPPORTED_RUNTIME")
		}
	}
}

const requiredScreen = `{"id": "t.req", "title": "Req", "script": "data me = demo.profile() required\ndata offers = demo.offers()", "timelines": [],
 "root": {"type": "stack", "id": "root", "children": []}}`

func TestDataSourceFailures(t *testing.T) {
	e := newEnv(t)
	e.publish(requiredScreen)
	good := e.srv.Ops["demo.profile"]

	e.srv.Ops["demo.profile"] = func(context.Context, ops.Req) (ops.Result, error) { return ops.Result{}, errors.New("db down") }
	code, _, b := e.do("GET", "/v1/screens/t.req", nil)
	wantErr(t, code, b, 503, "DATA_REQUIRED_FAILED")
	if get(obj(t, b), "error", "retryable") != true {
		t.Fatalf("retryable: %s", b)
	}

	e.srv.Ops["demo.profile"] = good
	e.srv.Ops["demo.offers"] = func(ctx context.Context, _ ops.Req) (ops.Result, error) {
		time.Sleep(3 * time.Second) // ignores ctx: the resolver must not wait for it
		return ops.OK([]any{}), nil
	}
	start := time.Now()
	code, _, b = e.do("GET", "/v1/screens/t.req", nil)
	if code != 200 || get(obj(t, b), "data", "offers", "ok") != false || get(obj(t, b), "data", "offers", "error", "code") != "DATA_SOURCE_FAILED" {
		t.Fatalf("slow optional source: %d %s", code, b)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("per-source timeout not enforced: %v", d)
	}
}

func TestDataEndpoint(t *testing.T) {
	e := newEnv(t)
	rev := e.publish(promoScreen)
	code, _, b := e.do("POST", "/v1/data", map[string]any{"screen": "t.promo", "revision": rev, "params": map[string]any{"code": "WB25"}, "sources": []string{"promo"}})
	m := obj(t, b)
	if code != 200 || get(m, "promo", "value", "discount") != 25 || m["me"] != nil {
		t.Fatalf("%d %s", code, b)
	}
	_, _, b = e.do("POST", "/v1/data", map[string]any{"screen": "t.promo", "revision": rev})
	if m := obj(t, b); get(m, "me", "ok") != true || get(m, "promo", "ok") != true {
		t.Fatalf("no sources = all: %s", b)
	}
	code, _, b = e.do("POST", "/v1/data", map[string]any{"screen": "t.promo", "revision": 99, "sources": []string{"promo"}})
	wantErr(t, code, b, 409, "REVISION_NOT_FOUND")
	code, _, b = e.do("POST", "/v1/data", map[string]any{"screen": "t.promo", "revision": rev, "sources": []string{"nope"}})
	wantErr(t, code, b, 400, "ARGS_INVALID")
	code, _, b = e.do("POST", "/v1/data", map[string]any{"screen": "t.promo", "revision": rev, "params": map[string]any{"code": 5}})
	if code != 200 || get(obj(t, b), "promo", "error", "code") != "ARGS_INVALID" {
		t.Fatalf("a param of the wrong type fails its source at the trust boundary: %d %s", code, b)
	}
}

func TestCall(t *testing.T) {
	e := newEnv(t)
	rev := e.publish(promoScreen)
	call := func(op string, args any, key string, hdr ...string) (int, map[string]any) {
		t.Helper()
		body := map[string]any{"screen": "t.promo", "revision": rev, "op": op, "args": args}
		if key != "" {
			body["idempotencyKey"] = key
		}
		code, _, b := e.do("POST", "/v1/call", body, hdr...)
		return code, obj(t, b)
	}
	errCode := func(m map[string]any) any { return get(m, "error", "code") }

	if code, m := call("demo.checkPromo", map[string]any{"code": "WB10"}, ""); code != 200 || get(m, "value", "valid") != true {
		t.Fatalf("bound read op: %d %v", code, m)
	}
	if code, m := call("demo.profile", map[string]any{}, ""); code != 403 || errCode(m) != "OPERATION_NOT_BOUND" {
		t.Fatalf("op used only by data: %d %v", code, m)
	}
	if code, m := call("demo.nope", map[string]any{}, ""); code != 400 || errCode(m) != "OPERATION_UNKNOWN" {
		t.Fatalf("unknown op: %d %v", code, m)
	}
	code, _, b := e.do("POST", "/v1/call", map[string]any{"screen": "t.promo", "revision": 99, "op": "demo.checkPromo", "args": map[string]any{"code": "x"}})
	wantErr(t, code, b, 409, "REVISION_NOT_FOUND")
	for _, args := range []any{map[string]any{"code": 5}, map[string]any{}, map[string]any{"code": "x", "extra": 1}, []any{}} {
		if code, m := call("demo.checkPromo", args, ""); code != 400 || errCode(m) != "ARGS_INVALID" {
			t.Fatalf("args %v: %d %v", args, code, m)
		}
	}

	// idempotency
	if code, m := call("demo.claimOffer", map[string]any{"id": "o1"}, ""); code != 400 || errCode(m) != "IDEMPOTENCY_REQUIRED" {
		t.Fatalf("write without key: %d %v", code, m)
	}
	auth := []string{"Authorization", "Bearer user-1"}
	if code, m := call("demo.claimOffer", map[string]any{"id": "o1"}, "k1", auth...); code != 200 || get(m, "value", "tickets") != 15 {
		t.Fatalf("claim: %d %v", code, m)
	}
	if code, m := call("demo.claimOffer", map[string]any{"id": "o1"}, "k1", auth...); code != 200 || get(m, "value", "tickets") != 15 {
		t.Fatalf("replay must return the stored response: %d %v", code, m)
	}
	if _, _, b := e.do("GET", "/v1/screens/t.promo", nil, auth...); get(obj(t, b), "data", "me", "value", "tickets") != 15 {
		t.Fatalf("replay must not call the handler again: %s", b)
	}
	if code, m := call("demo.claimOffer", map[string]any{"id": "o2"}, "k1", auth...); code != 409 || errCode(m) != "IDEMPOTENCY_MISMATCH" {
		t.Fatalf("same key, other args: %d %v", code, m)
	}
	if code, m := call("demo.claimOffer", map[string]any{"id": "o1"}, "k1", "Authorization", "Bearer user-2"); code != 409 || errCode(m) != "IDEMPOTENCY_MISMATCH" {
		t.Fatalf("same key, other user: %d %v", code, m)
	}

	// domain error = 200 ok:false, stored like any response
	e.srv.Ops["demo.subscribe"] = func(context.Context, ops.Req) (ops.Result, error) {
		r := ops.Fail("ARGS_INVALID", "Проверьте поля формы")
		r.FieldErrors = map[string]string{"email": "Этот email уже занят"}
		return r, nil
	}
	sub := map[string]any{"email": "taken@example.com", "name": "I", "plan": "pro", "promo": ""}
	if code, m := call("demo.subscribe", sub, "k2"); code != 200 || m["ok"] != false || get(m, "fieldErrors", "email") != "Этот email уже занят" || get(m, "error", "retryable") != false {
		t.Fatalf("domain error: %d %v", code, m)
	}

	// in flight
	started, release := make(chan struct{}), make(chan struct{})
	e.srv.Ops["demo.claimOffer"] = func(context.Context, ops.Req) (ops.Result, error) {
		close(started)
		<-release
		return ops.OK(map[string]any{"tickets": 1}), nil
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		if code, m := call("demo.claimOffer", map[string]any{"id": "o3"}, "k3"); code != 200 || get(m, "value", "tickets") != 1 {
			t.Errorf("first call: %d %v", code, m)
		}
	})
	<-started
	if code, m := call("demo.claimOffer", map[string]any{"id": "o3"}, "k3"); code != 409 || errCode(m) != "CALL_IN_FLIGHT" || get(m, "error", "retryable") != true {
		t.Fatalf("in flight: %d %v", code, m)
	}
	close(release)
	wg.Wait()

	// a failing handler releases the key
	e.srv.Ops["demo.claimOffer"] = func(context.Context, ops.Req) (ops.Result, error) { return ops.Result{}, errors.New("boom") }
	if code, m := call("demo.claimOffer", map[string]any{"id": "o4"}, "k4"); code != 500 || errCode(m) != "INTERNAL" {
		t.Fatalf("handler failure: %d %v", code, m)
	}
	e.srv.Ops["demo.claimOffer"] = func(context.Context, ops.Req) (ops.Result, error) { return ops.OK(map[string]any{"tickets": 2}), nil }
	if code, m := call("demo.claimOffer", map[string]any{"id": "o4"}, "k4"); code != 200 || get(m, "value", "tickets") != 2 {
		t.Fatalf("retry after failure: %d %v", code, m)
	}
}

func TestTheme(t *testing.T) {
	e := newEnv(t)
	raw, _ := os.ReadFile(filepath.Join(contractDir, "tokens", "korpu.json"))
	sum := sha256.Sum256(raw)
	version := hex.EncodeToString(sum[:])[:16]

	code, hdr, b := e.do("GET", "/v1/theme?brand=korpu", nil)
	m := obj(t, b)
	if code != 200 || m["version"] != version || m["brand"] != "korpu" || get(m, "colors", "bg1", "light") == nil || hdr.Get("ETag") != `"`+version+`"` {
		t.Fatalf("%d %v %v", code, m["version"], m["brand"])
	}
	if code, _, b := e.do("GET", "/v1/theme?brand=korpu", nil, "If-None-Match", version); code != 304 || len(b) != 0 {
		t.Fatalf("If-None-Match: %d %s", code, b)
	}
	if _, _, b := e.do("GET", "/v1/theme?brand=kz", nil); obj(t, b)["brand"] != "kz" {
		t.Fatal("kz theme")
	}
	if _, _, b := e.do("GET", "/v1/theme?brand=acme", nil); obj(t, b)["version"] != version {
		t.Fatal("unknown brand falls back to korpu")
	}
}

func TestAssets(t *testing.T) {
	e := newEnv(t)
	if code, _, b := e.do("GET", "/assets/hello.txt", nil); code != 200 || string(b) != "hello asset" {
		t.Fatalf("%d %s", code, b)
	}
	if code, _, b := e.do("GET", "/assets/sub/x.txt", nil); code != 200 || string(b) != "x" {
		t.Fatalf("nested asset: %d %s", code, b)
	}
	for _, p := range []string{"/assets/nope.png", "/assets/", "/assets/sub", "/assets/sub/"} {
		code, _, b := e.do("GET", p, nil)
		wantErr(t, code, b, 404, "NOT_FOUND")
	}
}

const htmlScreen = `{"id": "t.html", "title": "<&>", "script": "", "timelines": [], "root": {"type": "stack", "id": "root", "children": [{"type": "text", "id": "t", "props": {"text": "a < b && c > d"}}]}}`

func TestServedDocumentMatchesETag(t *testing.T) {
	e := newEnv(t)
	e.publish(htmlScreen)
	_, _, b := e.do("GET", "/v1/screens/t.html", nil)
	var resp struct {
		ETag     string
		Revision int
		Document json.RawMessage
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(resp.Document)
	if want := fmt.Sprintf("%d-%x", resp.Revision, sum[:8]); resp.ETag != want || !bytes.Contains(resp.Document, []byte("a < b && c > d")) {
		t.Fatalf("served document bytes must be the hashed bytes: etag %s, hash of served %s\n%s", resp.ETag, want, resp.Document)
	}
}
