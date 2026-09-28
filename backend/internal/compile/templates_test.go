package compile

import (
	"strings"
	"testing"
)

func expect(t *testing.T, doc map[string]any, want map[string]string) {
	t.Helper()
	for path, w := range want {
		if got := js(at(doc, path)); got != w {
			t.Errorf("%s:\n got %s\nwant %s", path, got, w)
		}
	}
}

func TestTemplateButton(t *testing.T) {
	root := `{"type": "stack", "id": "root", "children": [
	  {"type": "Button", "id": "buy", "props": {"title": "=\"Buy \" + str(n)", "opacity": 0.5},
	   "on": {"tap": "n = n + 1", "appear": "n = 0"}, "when": "n < 10", "animate": {"opacity": "@motion.snappy"}},
	  {"type": "Button", "id": "b2", "props": {"title": "Two", "kind": "secondary"}}
	]}`
	doc, _ := compileOK(t, src("state n = 0", root, ""))
	expect(t, doc, map[string]string{
		"root/c/0/t":           `"stack"`,
		"root/c/0/id":          `"buy"`,
		"root/c/0/p":           `{"opacity":["v",0.5],"padding":["v",8]}`,
		"root/c/0/when":        `["op","\u003c",["s","n"],["v",10]]`,
		"root/c/0/anim":        `{"opacity":["t","motion","snappy"]}`,
		"root/c/0/inst":        `{"state":{"Button.presses":["v",0]}}`,
		"root/c/0/on/tap":      `[["set","i","Button.presses",[],["op","+",["u","Button.double",[["i","Button.presses"]]],["v",1]]],["set","s","n",[],["op","+",["s","n"],["v",1]]],["do","play",[["v","buy/pulse"]],false]]`,
		"root/c/0/on/appear":   `[["set","s","n",[],["v",0]]]`,
		"root/c/0/c/0/id":      `"buy/bg"`,
		"root/c/0/c/0/p/fill":  `["if",["op","==",["v","primary"],["v","primary"]],["t","color","accentPrimary"],["t","color","bg2"]]`,
		"root/c/0/c/0/p/scale": `["if",["self","pressed"],["v",0.9],["v",1]]`,
		"root/c/0/c/0/on":      `{"longPress":[["do","back",[],false]]}`,
		"root/c/0/c/1/id":      `"buy/label"`,
		"root/c/0/c/1/p/text":  `["op","+",["op","+",["v","Buy "],["f","str",[["s","n"]]]],["v","!"]]`,
		"root/c/1/c/0/p/fill":  `["if",["op","==",["v","secondary"],["v","primary"]],["t","color","accentPrimary"],["t","color","bg2"]]`,
		"root/c/1/on/tap":      `[["set","i","Button.presses",[],["op","+",["u","Button.double",[["i","Button.presses"]]],["v",1]]],["do","play",[["v","b2/pulse"]],false]]`,
		"fns":                  `{"Button.double":{"body":["op","*",["l","x"],["v",2]],"params":["x"]}}`,
	})
	tls := doc["timelines"].(map[string]any)
	if got := js(tls["buy/pulse"]); got != `{"autoplay":false,"autoreverse":false,"duration":100,"repeat":1,"tracks":[{"keys":[{"t":0,"v":1},{"t":100,"v":0.95}],"prop":"scale","stagger":0,"target":"buy/bg"}]}` {
		t.Errorf("timeline buy/pulse: %s", got)
	}
	if len(tls) != 2 || tls["b2/pulse"] == nil {
		t.Errorf("timelines: %s", js(tls))
	}
}

// Hygiene: the template's repeat var and lambda param are renamed, so the substituted outer `item` is not captured.
func TestTemplateHygieneSlotDrivers(t *testing.T) {
	root := `{"type": "stack", "id": "root", "children": [
	  {"type": "stack", "id": "outer", "repeat": {"over": "names", "as": "item"}, "children": [
	    {"type": "Card", "id": "card", "props": {"items": "=[item]", "title": "=item"}, "children": [{"type": "text", "id": "extra", "props": {"text": "=item"}}]}
	  ]},
	  {"type": "text", "id": "t2", "props": {"text": "=str(scroll.card.y)"}}
	]}`
	doc, _ := compileOK(t, src(`state names = ["a"]`, root, ""))
	expect(t, doc, map[string]string{
		"root/c/0/c/0/t":             `"scroll"`,
		"root/c/0/c/0/id":            `"card"`,
		"root/c/0/c/0/c/0/id":        `"card/head"`,
		"root/c/0/c/0/c/0/p/text":    `["l","item"]`,
		"root/c/0/c/0/c/0/p/offsetY": `["op","/",["dr","scroll","card","y"],["v",10]]`,
		"root/c/0/c/0/c/1/id":        `"card/row"`,
		"root/c/0/c/0/c/1/rep":       `{"as":"Card#1.item","index":"Card#1.index","over":["list",[["l","item"]]]}`,
		"root/c/0/c/0/c/1/p/text":    `["op","+",["op","+",["l","Card#1.item"],["l","item"]],["f","join",[["f","map",[["list",[["l","item"]]],["lam",["Card#1.x"],["l","Card#1.x"]]]],["v",""]]]]`,
		"root/c/0/c/0/c/2/id":        `"extra"`,
		"root/c/0/c/0/c/2/p/text":    `["l","item"]`,
		"root/c/1/p/text":            `["f","str",[["dr","scroll","card","y"]]]`,
	})
}

// Instance state is template-qualified; a nested template receiving its parent's state reads it by the parent's name.
func TestTemplateInstanceState(t *testing.T) {
	root := `{"type": "stack", "id": "root", "children": [
	  {"type": "Toggle", "id": "t1", "on": {"change": "count = count + 1"}},
	  {"type": "Toggle", "id": "t2", "repeat": {"over": "[1, 2]", "as": "k"}, "props": {"label": "=str(k)"}}
	]}`
	doc, _ := compileOK(t, src("state count = 0", root, ""))
	expect(t, doc, map[string]string{
		"root/c/0/inst":          `{"state":{"Toggle.on":["v",false]}}`,
		"root/c/0/on/tap":        `[["set","i","Toggle.on",[],["un","!",["i","Toggle.on"]]],["set","s","count",[],["op","+",["s","count"],["v",1]]]]`,
		"root/c/0/c/0/id":        `"t1/dot"`,
		"root/c/0/c/0/t":         `"shape"`,
		"root/c/0/c/0/inst":      `{"state":{"Dot.hover":["v",false]}}`,
		"root/c/0/c/0/p/opacity": `["if",["i","Toggle.on"],["v",1],["v",0.3]]`,
		"root/c/0/c/0/p/scale":   `["if",["i","Dot.hover"],["v",1.1],["v",1]]`,
		"root/c/1/rep":           `{"as":"k","index":"index","over":["list",[["v",1],["v",2]]]}`,
		"root/c/1/on/tap":        `[["set","i","Toggle.on",[],["un","!",["i","Toggle.on"]]]]`,
		"root/c/1/c/0/id":        `"t2/dot"`,
	})
}

// A template handler with params is inlined hygienically; emit without an instance handler disappears.
func TestTemplateHandlerInline(t *testing.T) {
	doc, meta := compileOK(t, src("", `{"type": "Row", "id": "r", "props": {"code": "x"}}`, ""))
	expect(t, doc, map[string]string{
		"root/t":      `"text"`,
		"root/id":     `"r"`,
		"root/p/text": `["v","x"]`,
		"root/on/tap": `[["let","Row#2.n",["v",2]],["let","Row#2.x",["op","+",["l","Row#2.n"],["v",1]]],["call","demo.checkPromo",["rec",{"code":["v","x"]}],"Row#2.r",true]]`,
		"handlers":    `{}`,
	})
	if js(meta.Ops) != `["demo.checkPromo"]` {
		t.Errorf("ops %v", meta.Ops)
	}
}

func TestTemplateErrors(t *testing.T) {
	cases := []struct{ name, root, path, want string }{
		{"missing param", `{"type": "Button", "id": "root"}`, "root", "title"},
		{"unknown prop", `{"type": "Button", "id": "root", "props": {"title": "a", "nope": 1}}`, "root/props/nope", "unknown prop"},
		{"param type", `{"type": "Button", "id": "root", "props": {"title": 1}}`, "root/props/title", "type"},
		{"param literal union", `{"type": "Button", "id": "root", "props": {"title": "a", "kind": "tertiary"}}`, "root/props/kind", "type"},
		{"self in instance", `{"type": "Button", "id": "root", "props": {"title": "=self.pressed ? \"a\" : \"b\""}}`, "root/props/title", "self.pressed"},
		{"self in modifier without tap", `{"type": "Dot", "id": "root", "props": {"lit": true, "scale": "=self.pressed ? 0.9 : 1"}}`, "root/props/scale", "self.pressed"},
		{"unknown event", `{"type": "Button", "id": "root", "props": {"title": "a"}, "on": {"change": "back()"}}`, "root/on/change", "event"},
		{"template event has no payload", `{"type": "Button", "id": "root", "props": {"title": "a"}, "on": {"tap": "track(\"x\", {e: event})"}}`, "root/on/tap", "event"},
		{"children without slot", `{"type": "Button", "id": "root", "props": {"title": "a"}, "children": [{"type": "spacer", "id": "s"}]}`, "root", "slot"},
		{"slot child error", `{"type": "Card", "id": "root", "props": {"items": "=[]"}, "children": [{"type": "text", "id": "s"}]}`, "root/children/0", "text"},
		{"instance id with slash", `{"type": "Dot", "id": "a/b", "props": {"lit": true}}`, "root/id", "/"},
	}
	for _, c := range cases {
		_, _, diags := Screen(contract(t), src("", c.root, ""), 1)
		found := false
		for _, d := range diags {
			if d.Path == c.path && strings.Contains(strings.ToLower(d.Msg), strings.ToLower(c.want)) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: diags %v, want path %q containing %q", c.name, diags, c.path, c.want)
		}
	}
}

func TestTemplateDefinitionErrors(t *testing.T) {
	c := contract(t)
	cases := []struct{ name, src, want string }{
		{"slot root", `{"id": "T", "title": "", "script": "", "root": {"type": "slot"}}`, "root"},
		{"two slots", `{"id": "T", "title": "", "script": "", "root": {"type": "stack", "id": "root", "children": [{"type": "slot"}, {"type": "slot"}]}}`, "one slot"},
		{"slot fields", `{"id": "T", "title": "", "script": "", "root": {"type": "stack", "id": "root", "children": [{"type": "slot", "id": "s"}]}}`, "slot"},
		{"id mismatch", `{"id": "Other", "title": "", "script": "", "root": {"type": "spacer", "id": "root"}}`, "file name"},
		{"data", `{"id": "T", "title": "", "script": "data me = demo.profile()", "root": {"type": "spacer", "id": "root"}}`, "data"},
		{"handler recursion", `{"id": "T", "title": "", "script": "handler a() { a() }", "root": {"type": "spacer", "id": "root"}}`, "recurs"},
		{"instance state read in a state initializer", `{"id": "T", "title": "", "script": "state a = 1\nstate b = a + 1", "root": {"type": "spacer", "id": "root"}}`, "cannot read state 'a'"},
		{"required param default ok", `{"id": "T", "title": "", "script": "param p: number\nparam q: string = \"\"", "root": {"type": "spacer", "id": "root"}}`, ""},
	}
	for _, x := range cases {
		_, diags := compileTemplate(c, "T", []byte(x.src))
		msgs := js(diags)
		if x.want == "" && len(diags) > 0 || x.want != "" && !strings.Contains(strings.ToLower(msgs), x.want) {
			t.Errorf("%s: diags %s, want %q", x.name, msgs, x.want)
		}
	}
	if _, err := loadContract(contractDir, "testdata/recursive_templates"); err == nil || !strings.Contains(err.Error(), "recurs") {
		t.Errorf("recursive templates: %v", err)
	}
	b := c.Templates["Button"]
	if b == nil || len(b.Params) != 2 || b.Params[0].Name != "title" || b.Params[0].Default != nil || string(b.Params[1].Default) != `["v","primary"]` || !hasKey(b.Events, "tap") || b.Slot {
		t.Errorf("Button signature: %+v", b)
	}
	if !c.Templates["Card"].Slot {
		t.Error("Card has a slot")
	}
}

// Instance node fields move onto the expanded root; drag on the instance enables dragEnd and drag.<id> drivers.
func TestTemplateInstanceFields(t *testing.T) {
	root := `{"type": "stack", "id": "root", "children": [
	  {"type": "Dot", "id": "d1", "props": {"lit": "=gone", "offsetX": "=drag.d1.x"}, "drag": {"axis": "x"},
	   "on": {"dragEnd": "gone = event.x > 100"}, "a11y": {"label": "\"dot\"", "role": "image"},
	   "transition": {"in": "fade"}, "matchId": "\"m\""}
	]}`
	doc, _ := compileOK(t, src("state gone = false", root, ""))
	expect(t, doc, map[string]string{
		"root/c/0/t":          `"shape"`,
		"root/c/0/id":         `"d1"`,
		"root/c/0/drag":       `{"axis":"x"}`,
		"root/c/0/p/offsetX":  `["dr","drag","d1","x"]`,
		"root/c/0/p/opacity":  `["if",["s","gone"],["v",1],["v",0.3]]`,
		"root/c/0/on/dragEnd": `[["set","s","gone",[],["op","\u003e",["get",["l","event"],"x"],["v",100]]]]`,
		"root/c/0/a11y":       `{"label":["v","dot"],"role":"image"}`,
		"root/c/0/tr":         `{"in":[{"k":"fade"}]}`,
		"root/c/0/match":      `["v","m"]`,
	})
}

// A nested template's event handler runs the outer template's handler, which emits the outer event; repeats at
// three levels (screen, Outer, Inner) all bind "item" and must stay distinct.
func TestTemplateNestedEmitAndRepeats(t *testing.T) {
	root := `{"type": "stack", "id": "root", "children": [
	  {"type": "Outer", "id": "o", "repeat": {"over": "xs", "as": "item"}, "props": {"label": "=item"}, "on": {"done": "n = n + 1"}}
	]}`
	doc, _ := compileOK(t, src("state xs = [\"a\"]\nstate n = 0", root, ""))
	expect(t, doc, map[string]string{
		"root/c/0/id":                 `"o"`,
		"root/c/0/rep":                `{"as":"item","index":"index","over":["s","xs"]}`,
		"root/c/0/c/0/id":             `"o/list"`,
		"root/c/0/c/0/rep":            `{"as":"Outer#1.item","index":"Outer#1.index","over":["list",[["l","item"]]]}`,
		"root/c/0/c/0/c/0/id":         `"o/in"`,
		"root/c/0/c/0/c/0/c/0/id":     `"o/in/t"`,
		"root/c/0/c/0/c/0/c/0/rep":    `{"as":"Outer#1.Inner#1.item","index":"Outer#1.Inner#1.index","over":["list",[["op","+",["l","Outer#1.item"],["l","item"]]]]}`,
		"root/c/0/c/0/c/0/c/0/p/text": `["op","+",["l","Outer#1.Inner#1.item"],["op","+",["l","Outer#1.item"],["l","item"]]]`,
		"root/c/0/c/1/id":             `"o/b"`,
		"root/c/0/c/1/on/tap":         `[["set","i","Button.presses",[],["op","+",["u","Button.double",[["i","Button.presses"]]],["v",1]]],["set","s","n",[],["op","+",["s","n"],["v",1]]],["do","play",[["v","o/b/pulse"]],false]]`,
		"root/c/0/c/1/c/1/p/text":     `["op","+",["l","item"],["v","!"]]`,
	})
	tl := doc["timelines"].(map[string]any)["o/b/pulse"]
	if got := js(at(tl, "tracks/0/target")); got != `"o/b/bg"` {
		t.Errorf("timeline o/b/pulse target: %s (timelines %s)", got, js(doc["timelines"]))
	}
}

// Controller ruling: self.pressed is allowed in an instance's modifier props when the expanded root is pressable.
func TestTemplateSelfInModifiers(t *testing.T) {
	root := `{"type": "stack", "id": "root", "children": [
	  {"type": "Button", "id": "b", "props": {"title": "a", "scale": "=self.pressed ? 0.97 : 1"}},
	  {"type": "Dot", "id": "d", "props": {"lit": true, "opacity": "=self.pressed ? 0.5 : 1"}, "on": {"tap": "back()"}}
	]}`
	doc, _ := compileOK(t, src("", root, ""))
	expect(t, doc, map[string]string{
		"root/c/0/p/scale":   `["if",["self","pressed"],["v",0.97],["v",1]]`,
		"root/c/1/p/opacity": `["if",["self","pressed"],["v",0.5],["v",1]]`,
		"root/c/1/on/tap":    `[["do","back",[],false]]`,
	})
}

// Typed template events: the payload is bound to a hygienic local at the emit site, so the instance handler's
// `event` cannot clash with the textField's own `event` inside the template.
func TestTemplateTypedEvents(t *testing.T) {
	root := `{"type": "stack", "id": "root", "children": [
	  {"type": "Field", "id": "f", "on": {"change": "email = event.value"}},
	  {"type": "Form", "id": "form", "on": {"submit": "last = event.text"}},
	  {"type": "Field", "id": "quiet"}
	]}`
	doc, _ := compileOK(t, src("state email = \"\"\nstate last = \"\"", root, ""))
	expect(t, doc, map[string]string{
		"root/c/0/t":             `"textField"`,
		"root/c/0/on/change":     `[["let","Field#2.event",["rec",{"value":["get",["l","event"],"value"]}]],["set","s","email",[],["get",["l","Field#2.event"],"value"]]]`,
		"root/c/1/c/0/id":        `"form/in"`,
		"root/c/1/c/0/on/change": `[["let","Form#3.Field#2.event",["rec",{"value":["get",["l","event"],"value"]}]],["set","i","Form.draft",[],["get",["l","Form#3.Field#2.event"],"value"]],["let","Form#4.event",["rec",{"text":["get",["l","Form#3.Field#2.event"],"value"]}]],["set","s","last",[],["get",["l","Form#4.event"],"text"]]]`,
		"root/c/2/on/change":     `[]`,
	})
	bad := map[string]string{
		`{"type": "Field", "id": "root", "on": {"change": "x = event.nope"}}`:                     "nope",
		`{"type": "Button", "id": "root", "props": {"title": "a"}, "on": {"tap": "x = event.x"}}`: "template event 'tap' has no payload",
	}
	for node, want := range bad {
		_, _, diags := Screen(contract(t), src("state x = 0", node, ""), 1)
		if len(diags) == 0 || diags[0].Path != "root/on/"+map[bool]string{true: "change", false: "tap"}[strings.Contains(node, "Field")] || !strings.Contains(diags[0].Msg, want) {
			t.Errorf("%s: diags %v, want %q", node, diags, want)
		}
	}
}

// Nodes whose `when` is the literal false after substitution are removed with their timeline tracks; an emptied
// timeline is dropped along with play/stop of it; a literal-true `when` is omitted.
func TestPruning(t *testing.T) {
	root := `{"type": "stack", "id": "root", "children": [
	  {"type": "Hint", "id": "h1", "props": {"text": "a", "show": false}},
	  {"type": "Hint", "id": "h2", "props": {"text": "b"}},
	  {"type": "text", "id": "off", "when": "false", "props": {"text": "x"}},
	  {"type": "text", "id": "on", "when": "true", "props": {"text": "y"}}
	]}`
	tls := `[{"name": "fade", "tracks": [{"target": "off", "prop": "opacity", "keys": [{"t": 0, "v": 0}]}]}]`
	doc, _ := compileOK(t, src("handler go() {\n if true { stop(fade) }\n play(fade)\n}", root, tls))
	expect(t, doc, map[string]string{
		"root/c/0/id":       `"h1"`,
		"root/c/0/c":        `null`,
		"root/c/0/on/tap":   `[["do","haptic",[["v","light"]],false]]`,
		"root/c/1/c/0/id":   `"h2/bubble"`,
		"root/c/1/c/0/when": `null`,
		"root/c/1/on/tap":   `[["do","play",[["v","h2/blink"]],false],["do","haptic",[["v","light"]],false]]`,
		"root/c/2/id":       `"on"`,
		"root/c/2/when":     `null`,
		"handlers/go/body":  `[["if",["v",true],[],[]]]`,
	})
	tl := doc["timelines"].(map[string]any)
	if len(tl) != 1 || tl["h2/blink"] == nil || js(at(tl["h2/blink"], "autoplay")) != "true" {
		t.Errorf("timelines: %s", js(tl))
	}
}
