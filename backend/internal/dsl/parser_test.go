package dsl

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// sx renders a parsed expression as an S-expression.
func sx(e Expr) string {
	switch e := e.(type) {
	case nil:
		return "<nil>"
	case *Lit:
		switch v := e.Val.(type) {
		case string:
			return strconv.Quote(v)
		case nil:
			return "null"
		default:
			return fmt.Sprint(v)
		}
	case *Interp:
		return "(str " + sxList(e.Parts) + ")"
	case *Ident:
		return e.Name
	case *TokenRef:
		return "@" + e.Group + "." + e.Name
	case *Member:
		return "(. " + sx(e.X) + " " + e.Name + ")"
	case *Index:
		return "([] " + sx(e.X) + " " + sx(e.I) + ")"
	case *Call:
		if len(e.Args) == 0 {
			return "(call " + sx(e.Fn) + ")"
		}
		return "(call " + sx(e.Fn) + " " + sxList(e.Args) + ")"
	case *Unary:
		return "(" + e.Op + " " + sx(e.X) + ")"
	case *Binary:
		return "(" + e.Op + " " + sx(e.X) + " " + sx(e.Y) + ")"
	case *Cond:
		return "(? " + sx(e.C) + " " + sx(e.A) + " " + sx(e.B) + ")"
	case *ListLit:
		return "[" + sxList(e.Elems) + "]"
	case *RecLit:
		var fs []string
		for _, f := range e.Fields {
			fs = append(fs, f.Name+": "+sx(f.X))
		}
		return "{" + strings.Join(fs, ", ") + "}"
	case *Lambda:
		return "(=> [" + strings.Join(e.Params, " ") + "] " + sx(e.Body) + ")"
	}
	return fmt.Sprintf("<%T>", e)
}

func sxList(es []Expr) string {
	var out []string
	for _, e := range es {
		out = append(out, sx(e))
	}
	return strings.Join(out, " ")
}

func sxStmts(ss []Stmt) string {
	var out []string
	for _, s := range ss {
		out = append(out, sxStmt(s))
	}
	return strings.Join(out, " ")
}

func sxStmt(s Stmt) string {
	switch s := s.(type) {
	case *Assign:
		return "(set " + sx(s.Target) + " " + sx(s.Value) + ")"
	case *LetStmt:
		return "(let " + s.Name + " " + sx(s.Value) + ")"
	case *CallStmt:
		r := "(call " + s.Op + " " + sx(s.Args)
		if s.Result != "" {
			r += " -> " + s.Result
		}
		if s.Await {
			r += " await"
		}
		return r + ")"
	case *IfStmt:
		return "(if " + sx(s.Cond) + " [" + sxStmts(s.Then) + "] [" + sxStmts(s.Else) + "])"
	case *AnimStmt:
		return "(anim " + sx(s.Motion) + " [" + sxStmts(s.Body) + "])"
	case *ExprStmt:
		if s.Await {
			return "(await " + sx(s.Call) + ")"
		}
		return "(do " + sx(s.Call) + ")"
	case *EmitStmt:
		return "(emit " + s.Name + ")"
	}
	return fmt.Sprintf("<%T>", s)
}

func sxDecl(d Decl) string {
	ann := func(t *Type) string {
		if t == nil {
			return ""
		}
		return ": " + t.String()
	}
	params := func(ps []ParamDef) string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Name+": "+p.T.String())
		}
		return "(" + strings.Join(out, ", ") + ")"
	}
	switch d := d.(type) {
	case *StateDecl:
		return "state " + d.Name + ann(d.Ann) + " = " + sx(d.Init)
	case *LetDecl:
		return "let " + d.Name + ann(d.Ann) + " = " + sx(d.Value)
	case *FnDecl:
		return "fn " + d.Name + params(d.Params) + ann(d.Ret) + " = " + sx(d.Body)
	case *HandlerDecl:
		return "handler " + d.Name + params(d.Params) + " [" + sxStmts(d.Body) + "]"
	case *DataDecl:
		r := "data " + d.Name + " = " + d.Op + "(" + sx(d.Args) + ")"
		if d.Required {
			r += " required"
		}
		return r
	case *ParamDecl:
		r := "param " + d.Name + ann(d.T)
		if d.Default != nil {
			r += " = " + sx(d.Default)
		}
		return r
	case *EventDecl:
		return "event " + d.Name
	}
	return fmt.Sprintf("<%T>", d)
}

func TestParseExpr(t *testing.T) {
	cases := []struct{ src, want string }{
		{"1 + 2 * 3", "(+ 1 (* 2 3))"},
		{"(1 + 2) * 3", "(* (+ 1 2) 3)"},
		{"2 - 3 - 4", "(- (- 2 3) 4)"},
		{"-2 * 3", "(* (- 2) 3)"},
		{"-7 % 3", "(% (- 7) 3)"},
		{"!a.b", "(! (. a b))"},
		{"a || b && c", "(|| a (&& b c))"},
		{"a == b < c", "(== a (< b c))"},
		{"a ?? b ?? c", "(?? a (?? b c))"},
		{"a ?? b || c", "(?? a (|| b c))"},
		{"1 == 1 ? 2 : 3 + 1", "(? (== 1 1) 2 (+ 3 1))"},
		{"a ? b : c ? d : e", "(? a b (? c d e))"},
		{"a ?? b ? c : d", "(? (?? a b) c d)"},
		{"300ms + 1.5s", "(+ 300 1500)"},
		{"xs.map(x => x * 2)", "(call (. xs map) (=> [x] (* x 2)))"},
		{"xs.reduce(0, (acc, x) => acc + x)", "(call (. xs reduce) 0 (=> [acc x] (+ acc x)))"},
		{"f(() => 1)", "(call f (=> [] 1))"},
		{"x => y => x", "(=> [x] (=> [y] x))"},
		{"(a)", "a"},
		{"a[1].b(c)[2]", "([] (call (. ([] a 1) b) c) 2)"},
		{"[1, 2,]", "[1 2]"},
		{"[]", "[]"},
		{`{a: 1, "b-c": [true, null], data: 2}`, `{a: 1, b-c: [true null], data: 2}`},
		{`"x${a + 1}y"`, `(str "x" (+ a 1) "y")`},
		{`"${a}"`, `(str a)`},
		{`"plain"`, `"plain"`},
		{"@color.bg1", "@color.bg1"},
		{"event.value", "(. event value)"},
		{"items\n  .map(x => x)", "(call (. items map) (=> [x] x))"},
		{"a +\n b", "(+ a b)"},
		{"x.state", "(. x state)"},
	}
	for _, c := range cases {
		e, diags := ParseExpr(c.src)
		if len(diags) > 0 {
			t.Errorf("%q: unexpected diags %v", c.src, diags)
			continue
		}
		if got := sx(e); got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

func TestParseExprErrors(t *testing.T) {
	cases := []struct {
		src  string
		want string
		pos  Pos
	}{
		{"1 +", "syntax error", Pos{1, 4}},
		{"(1", "expected ')'", Pos{1, 3}},
		{"a b", "syntax error", Pos{1, 3}},
		{"{a 1}", "expected ':'", Pos{1, 4}},
		{"state", "syntax error", Pos{1, 1}},
		{`"${}"`, "syntax error", Pos{1, 4}},
		{`"abc`, "unterminated string", Pos{1, 1}},
		{"f\n(1)", "unexpected '('", Pos{2, 1}},
		{strings.Repeat("(", 10000) + "1" + strings.Repeat(")", 10000), "nested too deeply", Pos{}},
		{strings.Repeat("-", 10000) + "1", "nested too deeply", Pos{}},
		{"1" + strings.Repeat(" + 1", 10000), "nested too deeply", Pos{}},
		{"a" + strings.Repeat(" ?? a", 10000), "nested too deeply", Pos{}},
		{strings.Repeat(`"${`, 2000) + strings.Repeat(`}"`, 2000), "nested too deeply", Pos{}},
		{`"a"` + strings.Repeat(".upper()", 600), "nested too deeply", Pos{}},
		{"a" + strings.Repeat("[0]", 600), "nested too deeply", Pos{}},
		{"a" + strings.Repeat(".b", 600), "nested too deeply", Pos{}},
		{"r" + strings.Repeat(".with({a: 1})", 600), "nested too deeply", Pos{}},
	}
	for _, c := range cases {
		_, diags := ParseExpr(c.src)
		if len(diags) == 0 || !strings.Contains(diags[0].Msg, c.want) {
			t.Errorf("%q: diags %v, want %q", c.src, diags, c.want)
			continue
		}
		if c.pos != (Pos{}) && (Pos{diags[0].Line, diags[0].Col}) != c.pos {
			t.Errorf("%q: error at %d:%d, want %v", c.src, diags[0].Line, diags[0].Col, c.pos)
		}
	}
}

func TestParseStmts(t *testing.T) {
	cases := []struct{ src, want string }{
		{"x = 1", "(set x 1)"},
		{"items[i].done = !items[i].done", "(set (. ([] items i) done) (! (. ([] items i) done)))"},
		{"let y = a + 1", "(let y (+ a 1))"},
		{"let r = await call demo.checkPromo({code: p})", "(call demo.checkPromo {code: p} -> r await)"},
		{"call demo.track()", "(call demo.track <nil>)"},
		{"await call demo.x({})", "(call demo.x {} await)"},
		{"if a { x = 1 } else if b { x = 2 } else { x = 3 }", "(if a [(set x 1)] [(if b [(set x 2)] [(set x 3)])])"},
		{"if x > 0 {\n  y = 1\n}\nelse {\n  y = 2\n}", "(if (> x 0) [(set y 1)] [(set y 2)])"},
		{"if r.ok { }", "(if (. r ok) [] [])"},
		{"animate(@motion.snappy) { x = 1\n y = 2 }", "(anim @motion.snappy [(set x 1) (set y 2)])"},
		{"await play(intro)", "(await (call play intro))"},
		{"emit tap", "(emit tap)"},
		{"add(2, 3)\nadd(1, 1)", "(do (call add 2 3)) (do (call add 1 1))"},
		{"a = 1; b = 2;", "(set a 1) (set b 2)"},
		{"", ""},
	}
	for _, c := range cases {
		ss, diags := ParseStmts(c.src)
		if len(diags) > 0 {
			t.Errorf("%q: unexpected diags %v", c.src, diags)
			continue
		}
		if got := sxStmts(ss); got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

func TestParseStmtErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"x = 1 y = 2", "newline"},
		{"1 + 2", "statement"},
		{"let r = call demo.x()", "await"},
		{"await x", "call"},
		{"if a x = 1", "expected '{'"},
		{"if a { x = 1", "expected '}'"},
		{"emit", "name"},
		{strings.Repeat("if true { ", 10000) + strings.Repeat("}", 10000), "nested too deeply"},
		{"if a { }" + strings.Repeat(" else if a { }", 600), "nested too deeply"},
	}
	for _, c := range cases {
		_, diags := ParseStmts(c.src)
		if len(diags) == 0 || !strings.Contains(diags[0].Msg, c.want) || !strings.Contains(diags[0].Msg, "syntax error") {
			t.Errorf("%q: diags %v, want %q", c.src, diags, c.want)
		}
	}
}

func TestParseScript(t *testing.T) {
	src := `state count: number = 0
let total = items.map(i => i.price).sum() // computed
fn tax(x: number): number = x * 0.2
fn twice(x: number) = x * 2
handler add(i: number, note: string) {
  count = count + i
}
data me = demo.profile()
data offer = demo.offer({id: id}) required
param id: string = ""
param mode: "a" | "b"
event tap`
	want := []string{
		"state count: number = 0",
		"let total = (call (. (call (. items map) (=> [i] (. i price))) sum))",
		"fn tax(x: number): number = (* x 0.2)",
		"fn twice(x: number) = (* x 2)",
		"handler add(i: number, note: string) [(set count (+ count i))]",
		"data me = demo.profile(<nil>)",
		"data offer = demo.offer({id: id}) required",
		`param id: string = ""`,
		`param mode: "a" | "b"`,
		"event tap",
	}
	s, diags := ParseScript(src)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags %v", diags)
	}
	if len(s.Decls) != len(want) {
		t.Fatalf("got %d decls, want %d", len(s.Decls), len(want))
	}
	for i, d := range s.Decls {
		if got := sxDecl(d); got != want[i] {
			t.Errorf("decl %d:\n got %s\nwant %s", i, got, want[i])
		}
	}
	if p := s.Decls[1].Pos(); p != (Pos{2, 5}) {
		t.Errorf("decl position %v, want 2:5", p)
	}
}

func TestParseScriptErrors(t *testing.T) {
	cases := []struct {
		src   string
		want  []string
		decls int
	}{
		{"state a = \nstate b = 1 +\nstate c = 2", []string{"syntax error", "syntax error"}, 1},
		{"state a = 1 state b = 2", []string{"newline"}, 1},
		{"foo = 1\nstate b = 2", []string{"declaration"}, 1},
		{"state let = 1", []string{"name"}, 0},
		{"state xs: any = 1", []string{"any"}, 1},
		{"fn f(x) = x", []string{"expected ':'"}, 0},
		{`handler h() { x = "a }`, []string{"unterminated string", "expected '}'"}, 0},
	}
	for _, c := range cases {
		s, diags := ParseScript(c.src)
		if len(diags) != len(c.want) {
			t.Errorf("%q: diags %v, want %d", c.src, diags, len(c.want))
			continue
		}
		for i, w := range c.want {
			if !strings.Contains(diags[i].Msg, w) {
				t.Errorf("%q: diag %d = %q, want %q", c.src, i, diags[i].Msg, w)
			}
		}
		if len(s.Decls) != c.decls {
			t.Errorf("%q: %d decls, want %d", c.src, len(s.Decls), c.decls)
		}
	}
}
