package dsl

import (
	"encoding/json"
	"testing"
)

func lowerEnv(t *testing.T) *Env {
	env := contractEnv(t)
	env.Timelines = map[string]bool{"intro": true}
	env.Drivers = func(kind, id string) bool { return kind == "scroll" && id == "list" }
	return env
}

func TestLowerExpr(t *testing.T) {
	script := `state n = 1
state xs: list<number> = []
state user = {name: "A"}
state m: map<number> = {}
state ms: list<string>? = null
let c = n * 2
data me = demo.profile()
param id: string = ""
fn twice(x: number): number = x * 2`
	sc := &Scope{Locals: map[string]*Type{"item": mustType(t, "{id: string}")}, Event: mustType(t, "{value: string}"), Self: true}
	cases := []struct{ expr, want string }{
		{"1 + 2 * 3", `["op","+",["v",1],["op","*",["v",2],["v",3]]]`},
		{"-n", `["un","-",["s","n"]]`},
		{"!true", `["un","!",["v",true]]`},
		{"300ms + 1.5s", `["op","+",["v",300],["v",1500]]`},
		{"0.2", `["v",0.2]`},
		{`"Hi, ${user.name}!"`, `["str",[["v","Hi, "],["get",["s","user"],"name"],["v","!"]]]`},
		{`"\${x} <&>"`, `["v","${x} <&>"]`},
		{"null ?? 5", `["op","??",["v",null],["v",5]]`},
		{"n > 0 ? c : 0", `["if",["op",">",["s","n"],["v",0]],["c","c"],["v",0]]`},
		{"xs.map((x, i) => x + i)", `["f","map",[["s","xs"],["lam",["x","i"],["op","+",["l","x"],["l","i"]]]]]`},
		{"map(xs, x => x)", `["f","map",[["s","xs"],["lam",["x"],["l","x"]]]]`},
		{`"abc".length`, `["f","length",[["v","abc"]]]`},
		{"xs.count", `["f","count",[["s","xs"]]]`},
		{"ms.first ?? \"\"", `["op","??",["f","first",[["s","ms"]]],["v",""]]`},
		{"min(n, 2)", `["f","min",[["s","n"],["v",2]]]`},
		{"twice(c)", `["u","twice",[["c","c"]]]`},
		{"me.name", `["get",["d","me"],"name"]`},
		{"id", `["p","id"]`},
		{"xs[0]", `["at",["s","xs"],["v",0]]`},
		{`m["k"]`, `["at",["s","m"],["v","k"]]`},
		{"@space.lg", `["t","space","lg"]`},
		{"@icon.close", `["t","icon","close"]`},
		{"env.now", `["e","now"]`},
		{"scroll.list.y", `["dr","scroll","list","y"]`},
		{"self.pressed", `["self","pressed"]`},
		{"item.id", `["get",["l","item"],"id"]`},
		{"event.value", `["get",["l","event"],"value"]`},
		{"[1, 2]", `["list",[["v",1],["v",2]]]`},
		{`{b: 1, a: "x"}`, `["rec",{"b":["v",1],"a":["v","x"]}]`},
		{"user.with({name: \"B\"})", `["f","with",[["s","user"],["rec",{"name":["v","B"]}]]]`},
	}
	for _, c := range cases {
		env := lowerEnv(t)
		e, _, diags := compile(t, env, script, c.expr, sc)
		if len(diags) > 0 {
			t.Errorf("%s: unexpected diags %v", c.expr, diags)
			continue
		}
		if got := string(LowerExpr(e, nil)); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.expr, got, c.want)
		}
	}
}

func TestLowerStmts(t *testing.T) {
	script := `state n = 0
state items = [{done: false}]
state promo = ""
state m: map<number> = {}
data me = demo.profile()
handler b() { }
handler add(x: number) { }`
	cases := []struct{ src, want string }{
		{"n = n + 1", `[["set","s","n",[],["op","+",["s","n"],["v",1]]]]`},
		{"items[0].done = true", `[["set","s","items",[["v",0],["v","done"]],["v",true]]]`},
		{`m["k"] = 1`, `[["set","s","m",[["v","k"]],["v",1]]]`},
		{"let y = 1\nn = y", `[["let","y",["v",1]],["set","s","n",[],["l","y"]]]`},
		{"if n > 0 { n = 1 } else if n < 0 { n = 2 } else { n = 3 }",
			`[["if",["op",">",["s","n"],["v",0]],[["set","s","n",[],["v",1]]],[["if",["op","<",["s","n"],["v",0]],[["set","s","n",[],["v",2]]],[["set","s","n",[],["v",3]]]]]]]`},
		{"if n > 0 { n = 1 }", `[["if",["op",">",["s","n"],["v",0]],[["set","s","n",[],["v",1]]],[]]]`},
		{"animate(@motion.snappy) { n = 1 }", `[["anim",["t","motion","snappy"],[["set","s","n",[],["v",1]]]]]`},
		{"let r = await call demo.checkPromo({code: promo})\nn = r.value.discount ?? 0",
			`[["call","demo.checkPromo",["rec",{"code":["s","promo"]}],"r",true],["set","s","n",[],["op","??",["get",["get",["l","r"],"value"],"discount"],["v",0]]]]`},
		{`call demo.claimOffer({id: "o1"})`, `[["call","demo.claimOffer",["rec",{"id":["v","o1"]}],null,false]]`},
		{"call demo.profile()", `[["call","demo.profile",["v",{}],null,false]]`},
		{`navigate("x", {}, "fade")`, `[["do","navigate",[["v","x"],["rec",{}],["v","fade"]],false]]`},
		{"play(intro)\nawait play(intro)\nstop(intro)", `[["do","play",[["v","intro"]],false],["do","play",[["v","intro"]],true],["do","stop",[["v","intro"]],false]]`},
		{"refresh(me)", `[["do","refresh",[["v","me"]],false]]`},
		{"wait(300ms)", `[["do","wait",[["v",300]],false]]`},
		{"back()", `[["do","back",[],false]]`},
		{"b()\nadd(2)", `[["run","b",[]],["run","add",[["v",2]]]]`},
		{"", `[]`},
	}
	for _, c := range cases {
		env := lowerEnv(t)
		_, prog, diags := compile(t, env, script, "", nil)
		ss, d := ParseStmts(c.src)
		diags = append(append(diags, d...), prog.CheckStmts(ss, nil)...)
		if len(diags) > 0 {
			t.Errorf("%s: unexpected diags %v", c.src, diags)
			continue
		}
		if got := string(LowerStmts(ss, nil)); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

func TestLowerProgram(t *testing.T) {
	env := lowerEnv(t)
	env.Ops["test.q"] = &Op{ID: "test.q", Kind: "read", Args: mustType(t, "{n: number, xs: list<number>, s: string, o: {id: string}}"), Result: TNumber}
	script := `param id: string = "x"
param req: number
data me = demo.profile()
data offer = demo.checkPromo({code: id}) required
data q = test.q({n: -1, xs: [1, 2.5], s: "a", o: {id: id}})
state page = 0
state last = page + 1
let isLast = page == 2
fn tax(x: number): number = x * 0.2
handler next(by: number) { page = min(page + by, 2) }`
	_, prog, diags := compile(t, env, script, "", nil)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags %v", diags)
	}
	got, err := json.Marshal(prog.Lower(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"params":{"id":["v","x"],"req":null},` +
		`"state":[["page",["v",0]],["last",["op","+",["s","page"],["v",1]]]],` +
		`"computed":{"isLast":["op","==",["s","page"],["v",2]]},` +
		`"fns":{"tax":{"params":["x"],"body":["op","*",["l","x"],["v",0.2]]}},` +
		`"handlers":{"next":{"params":["by"],"body":[["set","s","page",[],["f","min",[["op","+",["s","page"],["l","by"]],["v",2]]]]]}},` +
		`"data":[{"name":"me","op":"demo.profile","args":{},"required":false},` +
		`{"name":"offer","op":"demo.checkPromo","args":{"code":{"$param":"id"}},"required":true},` +
		`{"name":"q","op":"test.q","args":{"n":-1,"xs":[1,2.5],"s":"a","o":{"id":{"$param":"id"}}},"required":false}]}`
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestLowerTemplateHooks(t *testing.T) {
	env := lowerEnv(t)
	env.Template = true
	script := "param title: string\nevent tap\nstate on = false\nhandler press() {\n on = !on\n if on { emit tap }\n}"
	e, prog, diags := compile(t, env, script, `title + str(on)`, nil)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags %v", diags)
	}
	hooks := &Hooks{
		Ref: func(k RefKind, name string) json.RawMessage {
			if k == RefParam && name == "title" {
				return json.RawMessage(`["v","Buy"]`)
			}
			return nil
		},
		Emit: func(event string) []json.RawMessage {
			return []json.RawMessage{json.RawMessage(`["run","` + event + `Outer",[]]`), json.RawMessage(`["do","back",[],false]`)}
		},
	}
	if got, want := string(LowerExpr(e, hooks)), `["op","+",["v","Buy"],["f","str",[["i","on"]]]]`; got != want {
		t.Errorf("expr: got %s, want %s", got, want)
	}
	d := prog.Lower(nil)
	if got, want := string(d.Handlers["press"].Body), `[["set","i","on",[],["un","!",["i","on"]]],["if",["i","on"],[["emit","tap"]],[]]]`; got != want {
		t.Errorf("no hooks: got %s, want %s", got, want)
	}
	if got, want := string(prog.Lower(hooks).Handlers["press"].Body), `[["set","i","on",[],["un","!",["i","on"]]],["if",["i","on"],[["run","tapOuter",[]],["do","back",[],false]],[]]]`; got != want {
		t.Errorf("hooks: got %s, want %s", got, want)
	}
	if got, want := d.Events, []string{"tap"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("events: %v", got)
	}
	if got := string(d.Params["title"]); got != "null" {
		t.Errorf("required param default: %s", got)
	}
}
