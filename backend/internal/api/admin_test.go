package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"wishbone/backend/internal/compile"
	"wishbone/backend/internal/preview"
)

func TestAdminAuth(t *testing.T) {
	e := newEnv(t)
	for _, auth := range []string{"", "Bearer nope", "Basic " + password, "Bearer " + password + "x"} {
		code, _, b := e.do("GET", "/admin/api/screens", nil, "Authorization", auth)
		wantErr(t, code, b, 401, "UNAUTHENTICATED")
	}
	if code, _ := e.admin("GET", "/screens", nil); code != 200 {
		t.Fatalf("with password: %d", code)
	}
	if code, _, _ := e.do("GET", "/admin/api/screens?token="+password, nil); code != 401 {
		t.Fatal("?token= is only for WebSockets")
	}
}

const lifeV1 = `{"id": "t.life", "title": "Life", "script": "state n = 1", "timelines": [], "root": {"type": "stack", "id": "root", "children": [{"type": "text", "id": "a", "props": {"text": "=\"n=${n}\""}}]}}`
const lifeV2 = `{"id": "t.life", "title": "Life 2", "script": "", "timelines": [], "root": {"type": "stack", "id": "root", "children": []}}`

func TestAdminScreenLifecycle(t *testing.T) {
	e := newEnv(t)
	code, b := e.admin("POST", "/screens", map[string]string{"id": "t.life", "title": "Life"})
	m := obj(t, b)
	if code != 201 || m["id"] != "t.life" || get(m, "draft", "root", "type") != "stack" || get(m, "draft", "root", "children", 0, "type") != "text" {
		t.Fatalf("create: %d %s", code, b)
	}
	if code, b := e.admin("POST", "/screens/t.life/publish", nil); code != 200 {
		t.Fatalf("the default draft must compile: %d %s", code, b)
	}
	for _, id := range []string{"Bad Id", "", "-x", strings.Repeat("a", 65), "a/b"} {
		code, b := e.admin("POST", "/screens", map[string]string{"id": id, "title": "x"})
		wantErr(t, code, b, 400, "ARGS_INVALID")
	}
	if code, _ := e.admin("POST", "/screens", map[string]string{"id": "t.life", "title": "x"}); code != 409 {
		t.Fatalf("duplicate: %d", code)
	}

	broken := `{"id": "t.life", "title": "Life", "script": "state n = ", "timelines": [], "root": {"type": "stack", "id": "root"}}`
	if code, b := e.admin("PUT", "/screens/t.life", map[string]any{"draft": json.RawMessage(broken)}); code != 200 || obj(t, b)["updatedAt"] == nil {
		t.Fatalf("a draft with errors is saved: %d %s", code, b)
	}
	code, b = e.admin("POST", "/screens/t.life/publish", nil)
	if code != 422 || get(obj(t, b), "diags", 0, "path") != "script" || get(obj(t, b), "diags", 0, "line") != 1 {
		t.Fatalf("publish with errors: %d %s", code, b)
	}
	if code, b := e.admin("PUT", "/screens/t.life", map[string]any{"draft": 5}); code != 400 {
		t.Fatalf("a draft must be an object: %d %s", code, b)
	}

	e.admin("PUT", "/screens/t.life", map[string]any{"title": "Life", "draft": json.RawMessage(lifeV1)})
	code, b = e.admin("POST", "/screens/t.life/publish", nil)
	r2 := obj(t, b)
	if code != 200 || r2["revision"] != 2 || !strings.HasPrefix(r2["etag"].(string), "2-") {
		t.Fatalf("publish: %d %s", code, b)
	}
	e.admin("PUT", "/screens/t.life", map[string]any{"title": "Life 2", "draft": json.RawMessage(lifeV2)})
	e.admin("POST", "/screens/t.life/publish", nil)

	code, b = e.admin("GET", "/screens/t.life/revisions", nil)
	var revs []map[string]any
	_ = json.Unmarshal(b, &revs)
	if code != 200 || len(revs) != 3 || revs[0]["revision"] != 3.0 || revs[2]["revision"] != 1.0 || revs[0]["runtimeMin"] != "1.0.0" || revs[0]["publishedAt"] == nil || revs[0]["etag"] == nil {
		t.Fatalf("revisions: %d %s", code, b)
	}

	code, b = e.admin("POST", "/screens/t.life/rollback/2", nil)
	if code != 200 || obj(t, b)["revision"] != 4 {
		t.Fatalf("rollback: %d %s", code, b)
	}
	_, b = e.admin("GET", "/screens/t.life", nil)
	m = obj(t, b)
	if m["publishedRev"] != 4 || m["title"] != "Life 2" || !sameJSON(t, m["draft"], lifeV1) || m["updatedAt"] == nil {
		t.Fatalf("after rollback the old source is the draft: %s", b)
	}
	if _, _, b := e.do("GET", "/v1/screens/t.life", nil); obj(t, b)["revision"] != 4 {
		t.Fatalf("live revision after rollback: %s", b)
	}
	code, b = e.admin("POST", "/screens/t.life/rollback/99", nil)
	wantErr(t, code, b, 409, "REVISION_NOT_FOUND")

	_, b = e.admin("GET", "/screens", nil)
	var list []map[string]any
	_ = json.Unmarshal(b, &list)
	found := false
	for _, s := range list {
		found = found || (s["id"] == "t.life" && s["publishedRev"] == 4.0 && s["title"] == "Life 2" && s["updatedAt"] != nil)
	}
	if !found {
		t.Fatalf("list: %s", b)
	}

	other := strings.Replace(lifeV2, `"t.life"`, `"t.other"`, 1)
	e.admin("PUT", "/screens/t.life", map[string]any{"draft": json.RawMessage(other)})
	code, b = e.admin("POST", "/screens/t.life/publish", nil)
	if code != 422 || get(obj(t, b), "diags", 0, "path") != "id" {
		t.Fatalf("the source id must match the screen: %d %s", code, b)
	}

	for _, p := range []string{"/screens/t.none", "/screens/t.none/revisions"} {
		code, b := e.admin("GET", p, nil)
		wantErr(t, code, b, 404, "SCREEN_NOT_FOUND")
	}
	code, b = e.admin("POST", "/screens/t.none/publish", nil)
	wantErr(t, code, b, 404, "SCREEN_NOT_FOUND")
	code, b = e.admin("PUT", "/screens/t.none", map[string]any{"draft": json.RawMessage(lifeV2)})
	wantErr(t, code, b, 404, "SCREEN_NOT_FOUND")
}

func sameJSON(t *testing.T, got any, want string) bool {
	var w any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(got, ints(w))
}

func TestAdminCompile(t *testing.T) {
	e := newEnv(t)
	code, b := e.admin("POST", "/compile", map[string]any{"source": json.RawMessage(lifeV1)})
	m := obj(t, b)
	if code != 200 || m["ok"] != true || len(m["diags"].([]any)) != 0 || get(m, "document", "screen") != "t.life" || get(m, "document", "revision") != 0 {
		t.Fatalf("%d %s", code, b)
	}
	bad := strings.Replace(lifeV1, `state n = 1`, `state n = nope`, 1)
	_, b = e.admin("POST", "/compile", map[string]any{"source": json.RawMessage(bad)})
	m = obj(t, b)
	if m["ok"] != false || m["document"] != nil || get(m, "diags", 0, "path") != "script" || get(m, "diags", 0, "col") != 11 || get(m, "diags", 0, "message") == nil {
		t.Fatalf("%s", b)
	}
}

func TestCatalog(t *testing.T) {
	e := newEnv(t)
	code, b := e.admin("GET", "/catalog", nil)
	if code != 200 {
		t.Fatalf("%d %s", code, b)
	}
	var c struct {
		Primitives struct{ Primitives map[string]any }
		Stdlib     struct{ Functions map[string]any }
		Templates  json.RawMessage
		Tokens     map[string][]string
		Icons      []string
		Ops        []map[string]string
		Brands     []string
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if c.Primitives.Primitives["text"] == nil || c.Stdlib.Functions["min"] == nil {
		t.Fatal("primitives.json / stdlib.json are served as is")
	}
	want := `[{"name":"Chip","params":[{"name":"title","type":"string"},{"name":"selected","type":"bool","default":false},{"name":"gap","type":"number","default":8},{"name":"tone","type":"\"a\" | \"b\"","default":"b"},{"name":"size","type":"number","default":-2},{"name":"pad","type":"number","default":"@space.md"},{"name":"sum","type":"number","default":"=1 + 2"}],"events":["tap"],"slot":false}]`
	if string(c.Templates) != want {
		t.Fatalf("templates\n got %s\nwant %s", c.Templates, want)
	}
	if !contains(c.Tokens["color"], "bg1") || !contains(c.Tokens["space"], "md") || len(c.Tokens["motion"]) == 0 || len(c.Tokens["type"]) == 0 || len(c.Tokens["radius"]) == 0 {
		t.Fatalf("tokens %v", c.Tokens)
	}
	if !contains(c.Icons, "close") || !reflect.DeepEqual(c.Brands, []string{"korpu", "kz"}) {
		t.Fatalf("icons %v brands %v", c.Icons, c.Brands)
	}
	found := false
	for _, op := range c.Ops {
		found = found || (op["id"] == "demo.subscribe" && op["kind"] == "write" && op["args"] == "{email: string, name: string, plan: string, promo: string}" && op["result"] == "{id: string, total: number}")
	}
	if !found || len(c.Ops) != 13 {
		t.Fatalf("ops %v", c.Ops)
	}

	code, b = e.admin("GET", "/templates/Chip", nil)
	if code != 200 || string(b) != chipV1 {
		t.Fatalf("template source: %d %s", code, b)
	}
	code, b = e.admin("GET", "/templates/..%2Fscreens%2Ft.seed", nil)
	wantErr(t, code, b, 404, "NOT_FOUND")
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if x == y {
			return true
		}
	}
	return false
}

func TestAdminSPA(t *testing.T) {
	e := newEnv(t)
	for path, want := range map[string]string{"/admin/": "<!doctype html>admin", "/admin/screens/x": "<!doctype html>admin", "/admin/assets/app.js": "console.log(1)"} {
		if code, _, b := e.do("GET", path, nil); code != 200 || string(b) != want {
			t.Errorf("%s: %d %s", path, code, b)
		}
	}
	if code, _, b := e.do("GET", "/", nil); code != 200 || string(b) != "<!doctype html>admin" { // redirected to /admin/
		t.Errorf("bare address: %d %s", code, b)
	}
	if code, _, _ := e.do("GET", "/nope", nil); code != 404 {
		t.Errorf("unknown path: %d", code)
	}
	e.srv.AdminDir = t.TempDir() + "/missing"
	if code, _, b := e.do("GET", "/admin/", nil); code != 404 || !strings.Contains(string(b), "npm run build") {
		t.Fatalf("missing build: %d %s", code, b)
	}
	code, _, b := e.do("GET", "/admin/api/nope", nil, "Authorization", "Bearer "+password)
	wantErr(t, code, b, 404, "NOT_FOUND")
	code, _, b = e.do("GET", "/admin/api/nope", nil)
	wantErr(t, code, b, 401, "UNAUTHENTICATED")
}

const (
	chipScreen  = `{"id": "t.chip", "title": "Chip", "script": "", "timelines": [], "root": {"type": "stack", "id": "root", "children": [{"type": "Chip", "id": "c", "props": {"title": "Hi"}}]}}`
	chip2Screen = `{"id": "t.chip2", "title": "Chip 2", "script": "", "timelines": [], "root": {"type": "stack", "id": "root", "children": [{"type": "Chip", "id": "c", "props": {"title": "Hi", "selected": true}}]}}`
)

func TestSeed(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// t.seed exists but was never published (a crash between create and publish)
	if code, b := e.admin("POST", "/screens", map[string]string{"id": "t.seed", "title": "Seed"}); code != 201 {
		t.Fatalf("%d %s", code, b)
	}
	for range 2 {
		if err := e.srv.Seed(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if code, _, b := e.do("GET", "/v1/screens/t.seed", nil); code != 200 || obj(t, b)["revision"] != 1 {
		t.Fatalf("a seeded screen without a revision is published, once: %d %s", code, b)
	}

	if code, b := e.admin("POST", "/screens/t.seed/unpublish", nil); code != 204 {
		t.Fatalf("unpublish: %d %s", code, b)
	}
	if err := e.srv.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if code, _, b := e.do("GET", "/v1/screens/t.seed", nil); code != 404 {
		t.Fatalf("a restart-like Seed must not republish an unpublished screen: %d %s", code, b)
	}
}

func TestUnpublish(t *testing.T) {
	e := newEnv(t)
	e.admin("POST", "/screens", map[string]string{"id": "t.slot", "title": "Slot"})
	if code, b := e.admin("POST", "/screens/t.slot/publish", nil); code != 200 {
		t.Fatalf("publish: %d %s", code, b)
	}
	if code, _, _ := e.do("GET", "/v1/screens/t.slot", nil); code != 200 {
		t.Fatalf("live before unpublish: %d", code)
	}
	if code, b := e.admin("POST", "/screens/t.slot/unpublish", nil); code != 204 {
		t.Fatalf("unpublish: %d %s", code, b)
	}
	code, _, b := e.do("GET", "/v1/screens/t.slot", nil)
	wantErr(t, code, b, 404, "SCREEN_NOT_FOUND")
	_, b = e.admin("GET", "/screens/t.slot", nil)
	if m := obj(t, b); m["unpublished"] != true || m["publishedRev"] == nil {
		t.Fatalf("admin view of an unpublished screen: %s", b)
	}
	if code, b := e.admin("POST", "/screens/t.slot/publish", nil); code != 200 {
		t.Fatalf("republish: %d %s", code, b)
	}
	if code, _, _ := e.do("GET", "/v1/screens/t.slot", nil); code != 200 {
		t.Fatalf("publish puts it back on the air: %d", code)
	}
	code, b = e.admin("POST", "/screens/nope/unpublish", nil)
	wantErr(t, code, b, 404, "SCREEN_NOT_FOUND")
}

// A screen file that does not parse or compile (e.g. mid-edit on disk) is logged and skipped, never fatal.
func TestSeedSkipsBadFiles(t *testing.T) {
	dir, err := makeContract(chipV1)
	if err != nil {
		t.Fatal(err)
	}
	for rel, src := range map[string]string{
		"screens/a.json": `{"id": "t.half", "title": "Half`,
		"screens/b.json": `{"id": "t.bad", "title": "Bad", "script": "state n = ", "timelines": [], "root": {"type": "stack", "id": "root"}}`,
		"screens/c.json": `{"id": "t.ok", "title": "Ok", "script": "", "timelines": [], "root": {"type": "stack", "id": "root", "children": []}}`,
	} {
		if err := write(dir, rel, src); err != nil {
			t.Fatal(err)
		}
	}
	newEnv(t) // empty tables
	e := newEnvWith(t, dir)
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	if err := e.srv.Seed(context.Background()); err != nil {
		t.Fatalf("seed must not fail on a bad screen file: %v", err)
	}
	for _, id := range []string{"t.seed", "t.ok"} {
		if code, _, b := e.do("GET", "/v1/screens/"+id, nil); code != 200 {
			t.Errorf("%s must still be seeded: %d %s", id, code, b)
		}
	}
	if code, b := e.admin("GET", "/screens/t.bad", nil); code != 404 {
		t.Errorf("a non-compiling seed file creates nothing: %d %s", code, b)
	}
	var errs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if m := obj(t, []byte(line)); m["level"] == "ERROR" {
			errs = append(errs, m)
		}
	}
	if len(errs) != 2 || !strings.HasSuffix(errs[0]["file"].(string), "a.json") || !strings.Contains(errs[0]["err"].(string), "unexpected end of JSON") ||
		!strings.HasSuffix(errs[1]["file"].(string), "b.json") || !strings.Contains(errs[1]["err"].(string), "script:1:11") {
		t.Fatalf("each bad file is logged at ERROR with its file and reason:\n%s", logs.String())
	}
}

func TestRecompile(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rev := e.publish(chipScreen)
	rev2 := e.publish(chip2Screen)
	live := func(e *env, id string) any {
		t.Helper()
		_, _, b := e.do("GET", "/v1/screens/"+id, nil)
		return obj(t, b)["revision"]
	}
	if err := e.srv.Recompile(ctx); err != nil {
		t.Fatal(err)
	}
	if live(e, "t.chip") != rev || live(e, "t.chip2") != rev2 {
		t.Fatal("unchanged templates must not create revisions")
	}

	// Chip changes its expansion (t.chip would republish) and drops `selected` (t.chip2 breaks)
	dir, err := makeContract(strings.Replace(chipV2, `param selected: bool = false\n`, "", 1))
	if err != nil {
		t.Fatal(err)
	}
	e3 := newEnvWith(t, dir)
	if err := e3.srv.Recompile(ctx); err == nil || !strings.Contains(err.Error(), "t.chip2") || strings.Contains(err.Error(), "t.chip ") {
		t.Fatalf("a screen that no longer compiles must stop the server: %v", err)
	}
	if live(e3, "t.chip") != rev {
		t.Fatal("nothing may be republished when any screen is broken")
	}

	v2, err := makeContract(chipV2)
	if err != nil {
		t.Fatal(err)
	}
	e2 := newEnvWith(t, v2)
	if err := e2.srv.Recompile(ctx); err != nil {
		t.Fatal(err)
	}
	_, _, b := e2.do("GET", "/v1/screens/t.chip", nil)
	if obj(t, b)["revision"] != rev+1 || !strings.Contains(string(b), `"op","+"`) || live(e2, "t.chip2") != rev2+1 {
		t.Fatalf("a changed template republishes its screens: %s", b)
	}
}

// An unpublished screen is off the air: Recompile must neither fail because it can no longer compile nor
// republish it (Publish would clear unpublished_at and put it back on the air).
func TestRecompileSkipsUnpublished(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rev := e.publish(chipScreen)
	if code, b := e.admin("POST", "/screens/t.chip/unpublish", nil); code != 204 {
		t.Fatalf("unpublish: %d %s", code, b)
	}
	v2, err := makeContract(strings.Replace(chipV2, `param selected: bool = false\n`, "", 1)) // drops a param t.chip's props use
	if err != nil {
		t.Fatal(err)
	}
	e2 := newEnvWith(t, v2)
	if err := e2.srv.Recompile(ctx); err != nil {
		t.Fatalf("an unpublished screen that no longer compiles must not stop Recompile: %v", err)
	}
	if code, _, _ := e2.do("GET", "/v1/screens/t.chip", nil); code != 404 {
		t.Fatalf("Recompile must not republish an unpublished screen: %d", code)
	}
	_, b := e2.admin("GET", "/screens/t.chip", nil)
	if m := obj(t, b); m["publishedRev"] != rev || m["unpublished"] != true {
		t.Fatalf("still unpublished at its old revision: %s", b)
	}
}

func TestNewRequiresErrorCodes(t *testing.T) {
	dir, err := makeContract(chipV1)
	if err != nil {
		t.Fatal(err)
	}
	if err := write(dir, "errors.json", `{"codes": {"INTERNAL": {"http": 500, "retryable": true}}}`); err != nil {
		t.Fatal(err)
	}
	c, err := compile.LoadContract(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{Contract: c, ContractDir: dir, Hub: preview.New()}); err == nil || !strings.Contains(err.Error(), "SCREEN_NOT_FOUND") {
		t.Fatalf("errors.json without a code the server uses: %v", err)
	}
}
