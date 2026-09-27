package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wishbone/backend/internal/compile"
	"wishbone/backend/internal/ops"
	"wishbone/backend/internal/preview"
	"wishbone/backend/internal/store"
)

var (
	testURL     = "postgres:///wishbone_api_test?sslmode=disable"
	contractDir string // the real contract's data files + test template/screen/asset, isolated from edits in contract/
)

const password = "secret"

// Chip is the test template; chipV2 changes its expansion, chipBroken drops a param the screens pass.
const (
	chipScript = `param title: string\nparam selected: bool = false\nparam gap: number = 8 // spacing\nparam tone: \"a\" | \"b\" = \"b\"\nparam size: number = -2\nparam pad: number = @space.md\nparam sum: number = 1 + 2\nevent tap`
	chipV1     = `{"id": "Chip", "title": "Chip", "script": "` + chipScript + `", "timelines": [], "root": {"type": "stack", "id": "root", "props": {"spacing": "=gap"}, "on": {"tap": "emit tap"}, "children": [{"type": "text", "id": "label", "props": {"text": "=title"}}]}}`
	chipV2     = `{"id": "Chip", "title": "Chip", "script": "` + chipScript + `", "timelines": [], "root": {"type": "stack", "id": "root", "props": {"spacing": "=gap + 1"}, "on": {"tap": "emit tap"}, "children": [{"type": "text", "id": "label", "props": {"text": "=title"}}]}}`
	chipBroken = `{"id": "Chip", "title": "Chip", "script": "event tap", "timelines": [], "root": {"type": "stack", "id": "root", "children": []}}`
	seedScreen = `{"id": "t.seed", "title": "Seed", "script": "", "timelines": [], "root": {"type": "stack", "id": "root", "children": [{"type": "text", "id": "hi", "props": {"text": "Привет"}}]}}`
)

func TestMain(m *testing.M) {
	if u := os.Getenv("WISHBONE_TEST_API_DATABASE_URL"); u != "" {
		testURL = u
	}
	ctx := context.Background()
	if err := store.DropDatabase(ctx, testURL); err != nil {
		fmt.Println("SKIP api tests: postgres unavailable:", err)
		os.Exit(0)
	}
	if err := store.EnsureDatabase(ctx, testURL); err != nil {
		panic(err)
	}
	var err error
	if contractDir, err = makeContract(chipV1); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = store.DropDatabase(ctx, testURL)
	os.Exit(code)
}

// makeContract copies the real contract's data files and adds a Chip template, a seed screen and an asset.
func makeContract(chip string) (string, error) {
	dir, err := os.MkdirTemp("", "wbcontract")
	if err != nil {
		return "", err
	}
	const real = "../../../contract"
	rels := []string{"primitives.json", "stdlib.json", "operations.json", "errors.json", "icons.json"}
	themes, _ := filepath.Glob(filepath.Join(real, "tokens", "*.json"))
	for _, f := range themes {
		rels = append(rels, filepath.Join("tokens", filepath.Base(f)))
	}
	for _, rel := range rels {
		b, err := os.ReadFile(filepath.Join(real, rel))
		if err != nil {
			return "", err
		}
		if err := write(dir, rel, string(b)); err != nil {
			return "", err
		}
	}
	for rel, s := range map[string]string{"templates/Chip.json": chip, "screens/t.seed.json": seedScreen, "assets/hello.txt": "hello asset"} {
		if err := write(dir, rel, s); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func write(dir, rel, s string) error {
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(s), 0o644)
}

type env struct {
	t   *testing.T
	srv *Server
	url string
}

func newEnv(t *testing.T) *env { return newEnvWith(t, contractDir) }

func newEnvWith(t *testing.T, dir string) *env {
	t.Helper()
	c, err := compile.LoadContract(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), testURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	admin := t.TempDir()
	_ = write(admin, "index.html", "<!doctype html>admin")
	_ = write(admin, "assets/app.js", "console.log(1)")
	srv, err := New(Config{Contract: c, ContractDir: dir, AdminDir: admin, Store: st,
		Ops: (&ops.Demo{NoLatency: true}).Handlers(), Hub: preview.New(), Password: password})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &env{t: t, srv: srv, url: ts.URL}
}

// do sends a request; hdr is name/value pairs. body: string = raw, nil = none, else JSON-encoded.
func (e *env) do(method, path string, body any, hdr ...string) (int, http.Header, []byte) {
	e.t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	default:
		j, err := json.Marshal(b)
		if err != nil {
			e.t.Fatal(err)
		}
		r = bytes.NewReader(j)
	}
	req, err := http.NewRequest(method, e.url+path, r)
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("X-Wishbone-Runtime", "1.0.0")
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i+1] == "" {
			req.Header.Del(hdr[i])
		} else {
			req.Header.Set(hdr[i], hdr[i+1])
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b
}

func (e *env) admin(method, path string, body any) (int, []byte) {
	e.t.Helper()
	code, _, b := e.do(method, "/admin/api"+path, body, "Authorization", "Bearer "+password)
	return code, b
}

// publish creates (or reuses) a screen with the given source as its draft and publishes it; returns the revision.
func (e *env) publish(src string) int {
	e.t.Helper()
	var head struct{ ID, Title string }
	if err := json.Unmarshal([]byte(src), &head); err != nil {
		e.t.Fatal(err)
	}
	if code, b := e.admin("POST", "/screens", map[string]string{"id": head.ID, "title": head.Title}); code != 201 && code != 409 {
		e.t.Fatalf("create %s: %d %s", head.ID, code, b)
	}
	if code, b := e.admin("PUT", "/screens/"+head.ID, map[string]any{"draft": json.RawMessage(src)}); code != 200 {
		e.t.Fatalf("save %s: %d %s", head.ID, code, b)
	}
	code, b := e.admin("POST", "/screens/"+head.ID+"/publish", nil)
	if code != 200 {
		e.t.Fatalf("publish %s: %d %s", head.ID, code, b)
	}
	return obj(e.t, b)["revision"].(int)
}

// obj decodes a JSON object; whole numbers become int for easy comparison.
func obj(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not a JSON object: %s", b)
	}
	return ints(m).(map[string]any)
}

func ints(v any) any {
	switch v := v.(type) {
	case float64:
		if v == float64(int(v)) {
			return int(v)
		}
	case map[string]any:
		for k, x := range v {
			v[k] = ints(x)
		}
	case []any:
		for i, x := range v {
			v[i] = ints(x)
		}
	}
	return v
}

// get walks a decoded JSON value: get(m, "data", "me", "value", "name").
func get(v any, path ...any) any {
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, _ := v.(map[string]any)
			v = m[k]
		case int:
			l, _ := v.([]any)
			if k >= len(l) {
				return nil
			}
			v = l[k]
		}
	}
	return v
}

func wantErr(t *testing.T, status int, b []byte, wantStatus int, code string) {
	t.Helper()
	if status != wantStatus || get(obj(t, b), "error", "code") != code {
		t.Fatalf("got %d %s, want %d %s", status, b, wantStatus, code)
	}
	if msg, _ := get(obj(t, b), "error", "message").(string); msg == "" {
		t.Fatalf("error without message: %s", b)
	}
}
