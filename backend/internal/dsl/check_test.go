package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const contractDir = "../../../contract"

func readContract(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(contractDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// contractEnv builds an Env from contract/: stdlib, operations, korpu theme tokens, icons.
func contractEnv(t testing.TB) *Env {
	t.Helper()
	std, err := LoadStdlib(readContract(t, "stdlib.json"))
	if err != nil {
		t.Fatal(err)
	}
	ops, err := LoadOps(readContract(t, "operations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var theme map[string]json.RawMessage
	if err := json.Unmarshal(readContract(t, "tokens/korpu.json"), &theme); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]map[string]bool{}
	for group, key := range map[string]string{"color": "colors", "type": "type", "space": "space", "radius": "radius", "motion": "motion"} {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(theme[key], &m); err != nil {
			t.Fatal(err)
		}
		tokens[group] = map[string]bool{}
		for name := range m {
			tokens[group][name] = true
		}
	}
	var icons map[string]json.RawMessage
	if err := json.Unmarshal(readContract(t, "icons.json"), &icons); err != nil {
		t.Fatal(err)
	}
	tokens["icon"] = map[string]bool{}
	for name := range icons {
		tokens["icon"][name] = true
	}
	return &Env{Stdlib: std, Ops: ops, Tokens: tokens}
}

// compile parses and checks script, then expr (if any) in its program.
func compile(t *testing.T, env *Env, script, expr string, sc *Scope) (Expr, *Program, []Diag) {
	t.Helper()
	s, diags := ParseScript(script)
	prog, d := Check(s, env)
	diags = append(diags, d...)
	if expr == "" {
		return nil, prog, diags
	}
	e, d := ParseExpr(expr)
	diags = append(diags, d...)
	if e != nil {
		diags = append(diags, prog.CheckExpr(e, sc, nil)...)
	}
	return e, prog, diags
}

func hasDiag(diags []Diag, sub string) bool {
	for _, d := range diags {
		if strings.Contains(strings.ToLower(d.Msg), strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

func TestLoadStdlib(t *testing.T) {
	std := contractEnv(t).Stdlib
	cases := map[string]string{
		"map":       "<T, U>(xs: list<T>, f: (T, number) => U) => list<U>",
		"contains":  "(s: string, t: string) => bool | <T>(xs: list<T>, v: T) => bool",
		"plural":    "(n: number, forms: {one?: string, few?: string, many?: string, other: string}) => string",
		"substring": "(s: string, start: number, end?: number) => string",
	}
	for name, want := range cases {
		if got := std.Funcs[name].String(); got != want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
	}
	if f := std.Funcs["isEmpty"]; !f.Method || !f.Property || len(f.Sigs) != 2 {
		t.Errorf("isEmpty flags/overloads wrong: %+v", f)
	}
	if got := std.Effects["refresh"].String(); got != "(source: <data name>)" {
		t.Errorf("refresh: %s", got)
	}
	if got := std.Env["platform"].String(); got != `"ios" | "android" | "test"` {
		t.Errorf("env.platform: %s", got)
	}
	if std.Easings["easeOut"] != [4]float64{0, 0, 0.58, 1} {
		t.Errorf("easeOut: %v", std.Easings["easeOut"])
	}
	if _, err := LoadStdlib([]byte(`{"functions": {"f": {"sig": "(x: nope) => number"}}}`)); err == nil {
		t.Error("bad sig must fail")
	}
	if _, err := LoadStdlib([]byte(`{"functions": {"f": {"sig": "(x: number) => number", "special": true}}}`)); err == nil {
		t.Error("unknown special function must fail")
	}
}

func TestCheckExprTypes(t *testing.T) {
	env := contractEnv(t)
	script := `state xs: list<number> = []
state user = {name: "Ann", age: 30}
state maybe: {a: number}? = null
state m: map<string> = {}
state flag = true
state ms: list<string>? = null
data me = demo.profile()
data offers = demo.offers()
param id: string = ""
fn twice(x: number): number = x * 2`
	cases := []struct{ expr, want string }{
		{"1 + 2 * 3", "number"},
		{`"a" + "b"`, "string"},
		{`"n=${1}"`, "string"},
		{"[1, null]", "list<number?>"},
		{"[null, 1]", "list<number?>"},
		{"xs.first", "number?"},
		{"xs[0]", "number?"},
		{"maybe.a", "number?"},
		{`m["k"]`, "string?"},
		{`m["k"] ?? ""`, "string"},
		{"null ?? 5", "number"},
		{"null ?? null ?? 3", "number"},
		{"maybe ?? {a: 1}", "{a: number}"},
		{"xs.first ?? maybe.a", "number?"},
		{"xs.first ?? -1", "number"},
		{"xs.map(x => str(x))", "list<string>"},
		{"xs.map((x, i) => x + i)", "list<number>"},
		{"map(xs, x => x > 1)", "list<bool>"},
		{"xs.reduce(0, (acc, x) => acc + x)", "number"},
		{"xs.find(x => x > 1)", "number?"},
		{"xs.sortBy(x => -x)", "list<number>"},
		{"@radius.pill", "number"},
		{"@motion.snappy", "motion"},
		{"@type.body", "textStyle"},
		{"@icon.close", "icon"},
		{"env.platform", `"ios" | "android" | "test"`},
		{"flag ? @color.el16 : gradient([@color.bg1, @color.bg2], 90)", "paint"},
		{`"abc".length`, "number"},
		{`"abc".isEmpty`, "bool"},
		{"xs.isEmpty", "bool"},
		{"isEmpty(xs)", "bool"},
		{"xs.count", "number"},
		{"ms.count", "number?"},
		{"ms.first", "string?"},
		{"ms.map(s => s.length)", "list<number>?"},
		{"offers.count ?? -1", "number"},
		{"user.with({age: 31})", "{name: string, age: number}"},
		{"maybe.with({a: 2})", "{a: number}?"},
		{"user.name", "string"},
		{"me.name", "string?"},
		{"twice(2)", "number"},
		{"id", "string"},
		{`formatDuration(1, "hms")`, "string"},
		{`num("1")`, "number?"},
		{"[1, 2].contains(2)", "bool"},
		{`"ab".contains("a")`, "bool"},
		{`contains("ab", "a")`, "bool"},
		{`{a: 1, b: "x"}`, "{a: number, b: string}"},
		{"range(3).map(i => {i: i})", "list<{i: number}>"},
		{"str([1])", "string"},
		{"alpha(@color.bg1, 0.5)", "color"},
		{"spring(300, 0.8)", "motion"},
		{"interpolate(1, [0, 1], [0, 1])", "number"},
		{`"a" < "b"`, "bool"},
		{"[1] == [1]", "bool"},
		{"maybe == null", "bool"},
		{"xs.first == 1", "bool"},
		{`env.colorScheme == "dark"`, "bool"},
		{`plural(2, {one: "a", other: "b"})`, "string"},
	}
	for _, c := range cases {
		e, _, diags := compile(t, env, script, c.expr, nil)
		if len(diags) > 0 {
			t.Errorf("%s: unexpected diags %v", c.expr, diags)
			continue
		}
		if got := e.Type().String(); got != c.want {
			t.Errorf("%s: type %s, want %s", c.expr, got, c.want)
		}
	}
}

func TestCheckExprErrors(t *testing.T) {
	env := contractEnv(t)
	script := `state xs: list<number> = []
state n: number? = null
state s = "x"
state rec = {a: 1}
handler h() { }
fn f(x: number): number = x`
	cases := []struct{ expr, want string }{
		{`"a" + 1`, "type"},
		{"!3", "type"},
		{"-true", "type"},
		{"1 ? 2 : 3", "type"},
		{`true ? 1 : "a"`, "type"},
		{`[1, "a"]`, "type"},
		{"n + 1", "type"},
		{"-n", "type"},
		{"xs[0] * 2", "type"},
		{"1 && true", "type"},
		{`1 == "1"`, "type"},
		{"1 < true", "type"},
		{"nope + 1", "unknown name"},
		{"frobnicate(1)", "unknown function"},
		{"@color.nope", "token"},
		{"@icon.nope", "icon"},
		{"@shade.x", "token"},
		{"(x => x)", "lambda"},
		{"f(y => y)", "lambda"},
		{"str(x => x)", "lambda"},
		{"xs.map((a, b, c) => a)", "lambda"},
		{"xs.reduce(0, x => x)", "lambda"},
		{"[]", "annot"},
		{`"abc".contains(1)`, "type"},
		{`xs.contains("a")`, "type"},
		{"n.isEmpty", "type"},
		{"count(xs.first)", "type"},
		{"rec.b", "unknown field"},
		{"xs.nope()", "unknown method"},
		{"s.upper", "must be called"},
		{"event.value", "event"},
		{"self.pressed", "self.pressed"},
		{"scroll.list.y", "unknown scroll driver"},
		{"scroll.list", "scroll"},
		{"env", "env"},
		{"env.nope", "unknown env"},
		{`formatDuration(1, "x")`, "type"},
		{`plural(1, {one: "a"})`, "other"},
		{`plural(1, {other: "a", zero: "b"})`, "zero"},
		{"min(1)", "arguments"},
		{"h()", "type error: handler"},
		{`navigate("x")`, "type error: effect"},
		{"rec.with({nope: 1})", "nope"},
		{"rec.with({a: true})", "type"},
		{"rec.with(rec)", "record literal"},
		{"[1].sortBy(x => true)", "type"},
		{`[1].sortBy(x => x > 0 ? 1 : "a")`, "sortBy keys must be all numbers or all strings, got number | string"},
		{`[1].sortBy(x => x > 0 ? 1 : "a")`, "type"},
		{`xs["a"]`, "type"},
		{"s[0]", "type"},
		{"xs.map(x => x.foo)", "unknown field"},
		{"{a: 1, a: 2}", "duplicate"},
		{"h", "not a value"},
		{"f", "not a value"},
		{"s(1)", "not a function"},
		{`"${h()}"`, "handler"},
	}
	for _, c := range cases {
		_, _, diags := compile(t, env, script, c.expr, nil)
		if !hasDiag(diags, c.want) {
			t.Errorf("%s: diags %v, want one containing %q", c.expr, diags, c.want)
		}
	}
}

func TestCheckScope(t *testing.T) {
	env := contractEnv(t)
	env.Drivers = func(kind, id string) bool { return kind == "scroll" && id == "list" }
	sc := &Scope{
		Locals: map[string]*Type{"item": mustType(t, "{id: string}"), "index": TNumber},
		Event:  mustType(t, "{value: string}"),
		Self:   true,
	}
	ok := []string{"item.id + str(index)", "event.value.trim()", "self.pressed", "scroll.list.y + 1", "[1].map(item => item + index)"}
	for _, src := range ok {
		if _, _, diags := compile(t, env, "", src, sc); len(diags) > 0 {
			t.Errorf("%s: unexpected diags %v", src, diags)
		}
	}
	bad := map[string]string{"drag.list.x": "unknown drag driver", "scroll.list.z": "axis", "event.nope": "unknown field"}
	for src, want := range bad {
		if _, _, diags := compile(t, env, "", src, sc); !hasDiag(diags, want) {
			t.Errorf("%s: diags %v, want %q", src, diags, want)
		}
	}
	// expected type is enforced
	e, _ := ParseExpr(`"x"`)
	prog, _ := Check(&Script{}, env)
	if diags := prog.CheckExpr(e, nil, TNumber); !hasDiag(diags, "type") {
		t.Errorf("want a type error, got %v", diags)
	}
	e, _ = ParseExpr(`"fill"`)
	if diags := prog.CheckExpr(e, nil, mustType(t, `number | "fill" | "wrap"`)); len(diags) > 0 {
		t.Errorf("literal against union: %v", diags)
	}
	ss, _ := ParseStmts("item.id = \"x\"")
	if diags := prog.CheckStmts(ss, sc); !hasDiag(diags, "assign") {
		t.Errorf("assign to repeat var: %v", diags)
	}
}

func TestCheckScriptOK(t *testing.T) {
	env := contractEnv(t)
	env.Timelines = map[string]bool{"intro": true}
	script := `param id: string = ""
data me = demo.profile()
data offer = demo.checkPromo({code: id}) required
state page = 0
state items: list<{id: number, qty: number}> = []
state mode: "a" | "b" = "a"
state m: map<number> = {a: 1}
let label = "${total} items"
let total = items.map(i => i.qty).sum()
fn twice(x: number) = tax(x) * 2
fn tax(x: number): number = x * 0.2
handler next(step: number) {
  animate(@motion.smooth) { page = min(page + step, 2) }
  mode = "b"
  items[0].qty = 1
  m["b"] = 2
  let r = await call demo.claimOffer({id: id})
  if r.ok { page = r.value.tickets ?? 0 } else if r.error.code == "X" { page = 0 } else { back() }
  refresh(me)
  wait(300ms)
  await play(intro)
  navigate("x", {id: id}, "push")
  track("t", {n: page, ok: r.ok, items: []})
}
handler go() { next(1) }`
	_, prog, diags := compile(t, env, script, "", nil)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags %v", diags)
	}
	want := map[string]string{"total": "number", "label": "string", "twice": "number", "offer": "{valid: bool, discount: number, message: string}", "me": "{name: string, tickets: number, avatar: string, level: number}?"}
	for _, d := range prog.Script.Decls {
		var got string
		switch d := d.(type) {
		case *LetDecl:
			got = d.T.String()
		case *FnDecl:
			got = d.Ret.String()
		case *DataDecl:
			got = d.T.String()
		default:
			continue
		}
		if w, ok := want[d.DeclName()]; ok && got != w {
			t.Errorf("%s: %s, want %s", d.DeclName(), got, w)
		}
	}
}

func TestCheckScriptErrors(t *testing.T) {
	cases := []struct{ name, script, want string }{
		{"assign computed", "state a = 1\nlet b = a\nhandler go() { b = 2 }", "assign"},
		{"assign param", "param p: string = \"\"\nhandler go() { p = \"x\" }", "assign"},
		{"assign data", "data me = demo.profile()\nhandler go() { me = null }", "assign"},
		{"assign local", "handler go(x: number) { x = 1 }", "assign"},
		{"assign fn", "fn f(): number = 1\nhandler go() { f = 1 }", "assign"},
		{"assign type", "state a = 1\nhandler go() { a = \"x\" }", "type"},
		{"assign literal union", "state mode: \"a\" | \"b\" = \"a\"\nhandler go() { mode = \"c\" }", "type"},
		{"assign path field", "state u = {a: 1}\nhandler go() { u.b = 1 }", "unknown field"},
		{"assign path type", "state u = {a: 1}\nhandler go() { u.a = true }", "type"},
		{"assign path index", "state xs = [1]\nhandler go() { xs[\"a\"] = 1 }", "type"},
		{"assign expr", "state n = 0\nhandler go() { n + 1 = 2 }", "assign"},
		{"recursion direct", "fn f(x: number): number = f(x)", "recurs"},
		{"recursion indirect", "fn f(x: number): number = g(x)\nfn g(x: number): number = f(x)", "recurs"},
		{"cycle", "let a = b + 1\nlet b = a + 1", "cycle"},
		{"self cycle", "let a = a + 1", "cycle"},
		{"state forward", "state a = b\nstate b = 1", "unknown"},
		{"state self", "state a = a", "unknown"},
		{"state sees computed", "state a = 1\nlet b = a\nstate c = b", "unknown"},
		{"fn sees state", "state a = 1\nfn f(): number = a", "unknown"},
		{"fn return type", "fn f(): string = 1", "type"},
		{"fn null return", "fn f() = null", "annot"},
		{"op unknown", "handler go() { call demo.nope({}) }", "operation"},
		{"op args", "handler go() { call demo.checkPromo({code: 1}) }", "type"},
		{"op args extra", "handler go() { call demo.checkPromo({code: \"a\", nope: 1}) }", "nope"},
		{"op args missing", "handler go() { call demo.checkPromo() }", "code"},
		{"data write", `data x = demo.subscribe({email: "", name: "", plan: "", promo: ""})`, "read"},
		{"data unknown op", "data x = demo.nope()", "operation"},
		{"data args state", "state s = \"\"\ndata x = demo.checkPromo({code: s})", "param"},
		{"data args expr", `data x = demo.checkPromo({code: "a" + "b"})`, "literals"},
		{"emit outside", "handler go() { emit tap }", "emit"},
		{"event decl outside", "event tap", "template"},
		{"lambda outside", "state f = (x => x)", "lambda"},
		{"empty list", "state xs = []", "annot"},
		{"null state", "state x = null", "annot"},
		{"null let", "handler go() { let x = null }", "annot"},
		{"duplicate", "state a = 1\nstate a = 2", "duplicate"},
		{"duplicate kinds", "state a = 1\nfn a(): number = 1", "duplicate"},
		{"duplicate param", "fn f(x: number, x: number): number = x", "duplicate"},
		{"duplicate local", "handler go() {\n let x = 1\n let x = 2\n}", "duplicate"},
		{"reserved", "state env = 1", "reserved"},
		{"handler in expr", "state n = 0\nhandler a() { }\nhandler go() { n = a() }", "type"},
		{"await effect", `handler go() { await navigate("x") }`, "await"},
		{"await handler", "handler a() { }\nhandler go() { await a() }", "await"},
		{"timeline unknown", "handler go() { play(nope) }", "timeline"},
		{"refresh non-data", "state s = 1\nhandler go() { refresh(s) }", "data"},
		{"unused call", "handler go() { min(1, 2) }", "unused"},
		{"effect args", `handler go() { navigate("a", {}, "slide") }`, "type"},
		{"wait type", `handler go() { wait("a") }`, "type"},
		{"animate type", "handler go() { animate(1) { } }", "type"},
		{"if cond", "handler go() { if 1 { } }", "type"},
		{"handler args", "handler a(x: number) { }\nhandler go() { a(\"s\") }", "type"},
		{"handler arity", "handler h(a: number) { }\nhandler go() { h() }", "arg"},
		{"param default refs", "state s = 1\nparam p: number = s", "unknown"},
		{"param default type", `param p: number = "x"`, "type"},
		{"color literal", `state c: color = "#FF0000"`, "type"},
		{"event decl value", "state x = tap\nevent tap", "event"},
		{"call result scope", "handler go() {\n if true { let r = await call demo.profile() }\n let y = r.ok\n}", "unknown name 'r'"},
		{"sibling lets ok", "state n = 0\nhandler go() {\n if true { let x = 1\n n = x } else { let x = 2\n n = x }\n let x = 3\n n = x\n}", "<no error>"},
		{"track args", `handler go() { track("t", {f: x => x}) }`, "lambda"},
		{"let shadows outer let", "state n = 0\nhandler h() {\n let x = 1\n if true { let x = \"a\" }\n n = x + 1\n}", "duplicate local"},
		{"let shadows param", "handler h(x: number) { if true { let x = 2 } }", "duplicate local"},
		{"let shadows in animate", "handler h() {\n let x = 1\n animate(@motion.snappy) { let x = 2 }\n}", "duplicate local"},
		{"call result shadows", "handler h() {\n let r = 1\n if true { let r = await call demo.profile() }\n}", "duplicate local"},
		{"driver in fn", "fn f(): number = scroll.list.y", "unknown name"},
		{"driver in state", "state s = scroll.list.y", "unknown name"},
		{"driver in param default", "param p: number = drag.card.x", "unknown name"},
		{"handler recursion direct", "handler h() { h() }", "recurs"},
		{"handler recursion indirect", "handler a() { b() }\nhandler b() { if true { a() } }", "recurs"},
	}
	for _, c := range cases {
		env := contractEnv(t)
		env.Drivers = func(kind, id string) bool { return true }
		_, _, diags := compile(t, env, c.script, "", nil)
		if c.want == "<no error>" && len(diags) > 0 || c.want != "<no error>" && !hasDiag(diags, c.want) {
			t.Errorf("%s: diags %v, want one containing %q", c.name, diags, c.want)
		}
	}
}

func TestCheckTemplate(t *testing.T) {
	env := contractEnv(t)
	env.Template = true
	script := "param title: string\nevent tap\nstate on = false\nhandler press() {\n on = !on\n emit tap\n}"
	_, prog, diags := compile(t, env, script, "", nil)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags %v", diags)
	}
	h := prog.Script.Decls[3].(*HandlerDecl)
	if a := h.Body[0].(*Assign); !a.Inst {
		t.Error("template state must be instance state")
	}
	if e, _ := ParseExpr("on"); len(prog.CheckExpr(e, nil, nil)) > 0 || e.(*Ident).Ref != RefInst {
		t.Error("template state read must be RefInst")
	}
	if _, _, diags := compile(t, env, "event tap\nhandler go() { emit nope }", "", nil); !hasDiag(diags, "emit") {
		t.Errorf("emit of undeclared event: %v", diags)
	}
}

// on.* bodies as in contract/conformance/events.json (payload types from primitives.json).
func TestCheckEventBodies(t *testing.T) {
	env := contractEnv(t)
	drag := &Scope{Event: mustType(t, "{x: number, y: number, vx: number, vy: number}")}
	change := &Scope{Event: mustType(t, "{value: string}")}
	repeat := &Scope{Locals: map[string]*Type{"p": mustType(t, "{id: string}"), "index": TNumber}, Event: mustType(t, "{}"), Self: true}
	cases := []struct {
		script, body string
		sc           *Scope
	}{
		{"state dismissed = false", "if abs(event.x) > 120 || abs(event.vx) > 800 { dismissed = true }", drag},
		{"state name = \"\"\nstate edits = 0", "name = event.value.trim()\nedits = edits + 1", change},
		{"state email = \"\"", "email = event.value", change},
		{"state picked = \"\"", "picked = p.id + str(index)", repeat},
		{"state n = 0\nhandler inc(by: number) { n = n + by }", "inc(3)", repeat},
	}
	for _, c := range cases {
		_, prog, diags := compile(t, env, c.script, "", nil)
		ss, d := ParseStmts(c.body)
		diags = append(append(diags, d...), prog.CheckStmts(ss, c.sc)...)
		if len(diags) > 0 {
			t.Errorf("%s: unexpected diags %v", c.body, diags)
		}
	}
	_, prog, _ := compile(t, env, "state s = 0", "", nil)
	ss, _ := ParseStmts("event = 1")
	if diags := prog.CheckStmts(ss, change); !hasDiag(diags, "assign") {
		t.Errorf("assign to event: %v", diags)
	}
}

// Overload trials must check each argument once (nested overloaded calls used to be exponential).
func TestCheckOverloadDepth(t *testing.T) {
	env := contractEnv(t)
	expr := "true"
	for i := 0; i < 40; i++ {
		expr = "isEmpty([" + expr + "])"
	}
	start := time.Now()
	e, _, diags := compile(t, env, "", "[1].contains(1) && "+expr, nil)
	if len(diags) > 0 || e.Type() != TBool {
		t.Fatalf("diags %v", diags)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("depth-40 overloaded call took %v", d)
	}
}
