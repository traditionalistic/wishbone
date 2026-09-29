package compile

import (
	"encoding/json"
	"strings"
	"testing"
)

const contractDir = "../../../contract"

var testC *Contract

func contract(t *testing.T) *Contract {
	t.Helper()
	if testC == nil {
		c, err := loadContract(contractDir, "testdata/templates")
		if err != nil {
			t.Fatal(err)
		}
		testC = c
	}
	return testC
}

// src builds a screen source; root and timelines are JSON fragments.
func src(script, root, timelines string) []byte {
	if timelines == "" {
		timelines = "[]"
	}
	s, _ := json.Marshal(script)
	return []byte(`{"id": "test", "title": "T", "script": ` + string(s) + `, "timelines": ` + timelines + `, "root": ` + root + `}`)
}

func compileOK(t *testing.T, source []byte) (map[string]any, Meta) {
	t.Helper()
	doc, meta, diags := Screen(contract(t), source, 3)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags %v", diags)
	}
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		t.Fatal(err)
	}
	return m, meta
}

// at digs into decoded JSON by a slash path of keys / indexes.
func at(v any, path string) any {
	for _, k := range strings.Split(path, "/") {
		switch x := v.(type) {
		case map[string]any:
			v = x[k]
		case []any:
			i := 0
			for _, ch := range k {
				i = i*10 + int(ch-'0')
			}
			if i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	return v
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestScreenDocument(t *testing.T) {
	script := `param id: string = "x"
data me = demo.profile()
state page = 0
let last = page == 2
fn tax(x: number): number = x * 0.2
handler next() { page = min(page + 1, 2)
 call demo.claimOffer({id: id}) }`
	root := `{"type": "stack", "id": "root", "props": {"spacing": "@space.lg", "padding": 16, "width": "fill", "background": "=last ? @color.bg1 : @color.bg2"},
	  "on": {"appear": "play(intro)"},
	  "children": [
	    {"type": "text", "id": "hero", "props": {"text": "=\"Hi, ${me.name ?? \"?\"}\"", "style": "@type.body"}},
	    {"type": "text", "id": "lit", "props": {"text": "\\=not an expr"}}
	  ]}`
	tls := `[{"name": "intro", "tracks": [{"target": "hero", "prop": "opacity", "keys": [{"t": 400, "v": 1, "ease": "easeOut"}, {"t": 0, "v": 0}]}]}]`
	doc, meta := compileOK(t, src(script, root, tls))
	want := map[string]string{
		"format":            `1`,
		"runtimeMin":        `"1.0.0"`,
		"screen":            `"test"`,
		"revision":          `3`,
		"params":            `{"id":["v","x"]}`,
		"state":             `[["page",["v",0]]]`,
		"computed/last":     `["op","==",["s","page"],["v",2]]`,
		"fns/tax/params":    `["x"]`,
		"data":              `[{"args":{},"name":"me","op":"demo.profile","required":false}]`,
		"timelines/intro":   `{"autoplay":false,"autoreverse":false,"duration":400,"repeat":1,"tracks":[{"keys":[{"t":0,"v":0},{"ease":[0,0,0.58,1],"t":400,"v":1}],"prop":"opacity","stagger":0,"target":"hero"}]}`,
		"root/t":            `"stack"`,
		"root/p/spacing":    `["t","space","lg"]`,
		"root/p/padding":    `["v",16]`,
		"root/p/width":      `["v","fill"]`,
		"root/p/background": `["if",["c","last"],["t","color","bg1"],["t","color","bg2"]]`,
		"root/on/appear":    `[["do","play",[["v","intro"]],false]]`,
		"root/c/0/p/text":   `["str",[["v","Hi, "],["op","??",["get",["d","me"],"name"],["v","?"]]]]`,
		"root/c/1/p/text":   `["v","=not an expr"]`,
		"root/c/1/id":       `"lit"`,
	}
	for path, w := range want {
		if got := js(at(doc, path)); got != w {
			t.Errorf("%s:\n got %s\nwant %s", path, got, w)
		}
	}
	if js(meta.Ops) != `["demo.claimOffer"]` || meta.RuntimeMin != "1.0.0" || len(meta.Data) != 1 || meta.Data[0].Name != "me" {
		t.Errorf("meta %+v", meta)
	}
}

func TestNodeFields(t *testing.T) {
	script := `state items = [{id: "a", on: true}]
state email = ""
state gone = false
handler pick(id: string) { }`
	root := `{"type": "scroll", "id": "list", "children": [
	  {"type": "stack", "id": "row", "repeat": {"over": "items", "as": "it", "key": "it.id"},
	   "when": "it.on && index >= 0",
	   "on": {"tap": "pick(it.id)"},
	   "props": {"scale": "=self.pressed ? 0.95 : 1", "offsetY": "=scroll.list.y / 2"},
	   "animate": {"scale": "@motion.snappy", "*": "spring(300, 0.8)"},
	   "transition": {"in": "fade + slide(bottom) + scale(0.9) + offset(0, -20)", "out": "fade"},
	   "matchId": "\"row-\" + it.id",
	   "a11y": {"label": "it.id", "role": "button", "hidden": false},
	   "children": [{"type": "text", "id": "t", "props": {"text": "=it.id + str(index)"}}]},
	  {"type": "textField", "id": "f", "props": {"bind": "email", "placeholder": "Email"}, "on": {"change": "gone = false"}},
	  {"type": "shape", "id": "card", "drag": {"axis": "x", "snap": [0, 300], "bounds": [-400, 400]}, "on": {"dragEnd": "gone = abs(event.x) > 120"},
	   "props": {"offsetX": "=drag.card.x"}}
	]}`
	doc, _ := compileOK(t, src(script, root, ""))
	want := map[string]string{
		"root/c/0/rep":        `{"as":"it","index":"index","key":["get",["l","it"],"id"],"over":["s","items"]}`,
		"root/c/0/when":       `["op","\u0026\u0026",["get",["l","it"],"on"],["op","\u003e=",["l","index"],["v",0]]]`,
		"root/c/0/on/tap":     `[["run","pick",[["get",["l","it"],"id"]]]]`,
		"root/c/0/p/scale":    `["if",["self","pressed"],["v",0.95],["v",1]]`,
		"root/c/0/p/offsetY":  `["op","/",["dr","scroll","list","y"],["v",2]]`,
		"root/c/0/anim":       `{"*":["f","spring",[["v",300],["v",0.8]]],"scale":["t","motion","snappy"]}`,
		"root/c/0/tr":         `{"in":[{"k":"fade"},{"edge":"bottom","k":"slide"},{"from":0.9,"k":"scale"},{"k":"offset","x":0,"y":-20}],"out":[{"k":"fade"}]}`,
		"root/c/0/match":      `["op","+",["v","row-"],["get",["l","it"],"id"]]`,
		"root/c/0/a11y":       `{"hidden":["v",false],"label":["get",["l","it"],"id"],"role":"button"}`,
		"root/c/0/c/0/p/text": `["op","+",["get",["l","it"],"id"],["f","str",[["l","index"]]]]`,
		"root/c/1/p/value":    `["s","email"]`,
		"root/c/1/on/change":  `[["set","s","email",[],["get",["l","event"],"value"]],["set","s","gone",[],["v",false]]]`,
		"root/c/2/drag":       `{"axis":"x","bounds":[-400,400],"snap":[0,300]}`,
		"root/c/2/on/dragEnd": `[["set","s","gone",[],["op","\u003e",["f","abs",[["get",["l","event"],"x"]]],["v",120]]]]`,
		"root/c/2/p/offsetX":  `["dr","drag","card","x"]`,
	}
	for path, w := range want {
		if got := js(at(doc, path)); got != w {
			t.Errorf("%s:\n got %s\nwant %s", path, got, w)
		}
	}
}

func TestScreenErrors(t *testing.T) {
	cases := []struct {
		name, script, root, timelines string
		path, want                    string
	}{
		{"unknown prop", "", `{"type": "text", "id": "root", "props": {"text": "a", "nope": 1}}`, "", "root/props/nope", "unknown prop"},
		{"missing required prop", "", `{"type": "text", "id": "root"}`, "", "root", "text"},
		{"prop type", "", `{"type": "text", "id": "root", "props": {"text": 1}}`, "", "root/props/text", "type"},
		{"color literal", "", `{"type": "text", "id": "root", "props": {"text": "a", "color": "#FF0000"}}`, "", "root/props/color", "type"},
		{"unknown token", "", `{"type": "text", "id": "root", "props": {"text": "a", "color": "@color.nope"}}`, "", "root/props/color", "token"},
		{"unknown icon", "", `{"type": "icon", "id": "root", "props": {"name": "@icon.nope"}}`, "", "root/props/name", "icon"},
		{"expr error", "", `{"type": "text", "id": "root", "props": {"text": "=1 +"}}`, "", "root/props/text", "syntax"},
		{"unknown event", "", `{"type": "text", "id": "root", "props": {"text": "a"}, "on": {"change": "back()"}}`, "", "root/on/change", "event"},
		{"dragEnd without drag", "", `{"type": "shape", "id": "root", "on": {"dragEnd": "back()"}}`, "", "root/on/dragEnd", "event"},
		{"unknown op", "", `{"type": "shape", "id": "root", "on": {"tap": "call demo.nope()"}}`, "", "root/on/tap", "operation"},
		{"event outside on", "", `{"type": "text", "id": "root", "props": {"text": "=event.value"}}`, "", "root/props/text", "event"},
		{"self without tap", "", `{"type": "shape", "id": "root", "props": {"scale": "=self.pressed ? 1 : 2"}}`, "", "root/props/scale", "self.pressed"},
		{"self in child", "", `{"type": "stack", "id": "root", "on": {"tap": "back()"}, "children": [{"type": "shape", "id": "s", "props": {"scale": "=self.pressed ? 1 : 2"}}]}`, "", "root/children/0/props/scale", "self.pressed"},
		{"unknown type", "", `{"type": "box", "id": "root"}`, "", "root", "unknown type"},
		{"unknown template", "", `{"type": "Nope", "id": "root"}`, "", "root", "template"},
		{"slot outside template", "", `{"type": "stack", "id": "root", "children": [{"type": "slot"}]}`, "", "root/children/0", "slot"},
		{"missing id", "", `{"type": "stack"}`, "", "root", "id"},
		{"duplicate id", "", `{"type": "stack", "id": "root", "children": [{"type": "spacer", "id": "a"}, {"type": "spacer", "id": "a"}]}`, "", "root/children/1/id", "duplicate id"},
		{"children on leaf", "", `{"type": "spacer", "id": "root", "children": [{"type": "spacer", "id": "a"}]}`, "", "root", "children"},
		{"unknown node key", "", `{"type": "spacer", "id": "root", "style": 1}`, "", "root", "style"},
		{"script error", "state a = ", `{"type": "spacer", "id": "root"}`, "", "script", "syntax"},
		{"when type", "", `{"type": "spacer", "id": "root", "when": "1"}`, "", "root/when", "type"},
		{"repeat over", "", `{"type": "spacer", "id": "root", "repeat": {"over": "1"}}`, "", "root/repeat/over", "list"},
		{"repeat key", "state xs: list<bool> = []", `{"type": "spacer", "id": "root", "repeat": {"over": "xs", "key": "item"}}`, "", "root/repeat/key", "type"},
		{"animate prop", "", `{"type": "spacer", "id": "root", "animate": {"text": "@motion.snappy"}}`, "", "root/animate/text", "animat"},
		{"animate type", "", `{"type": "spacer", "id": "root", "animate": {"scale": "1"}}`, "", "root/animate/scale", "type"},
		{"transition", "", `{"type": "spacer", "id": "root", "transition": {"in": "fade + spin"}}`, "", "root/transition/in", "spin"},
		{"transition edge", "", `{"type": "spacer", "id": "root", "transition": {"out": "slide(left)"}}`, "", "root/transition/out", "edge"},
		{"drag axis", "", `{"type": "spacer", "id": "root", "drag": {"axis": "z"}}`, "", "root/drag", "axis"},
		{"drag bounds", "", `{"type": "spacer", "id": "root", "drag": {"axis": "x", "bounds": [1]}}`, "", "root/drag", "bounds"},
		{"a11y role", "", `{"type": "spacer", "id": "root", "a11y": {"role": "link"}}`, "", "root/a11y/role", "role"},
		{"a11y label", "", `{"type": "spacer", "id": "root", "a11y": {"label": "1"}}`, "", "root/a11y/label", "type"},
		{"matchId", "", `{"type": "spacer", "id": "root", "matchId": "1"}`, "", "root/matchId", "type"},
		{"driver unknown", "", `{"type": "shape", "id": "root", "props": {"scale": "=scroll.root.y"}}`, "", "root/props/scale", "scroll driver"},
		{"drag driver needs drag", "", `{"type": "shape", "id": "root", "props": {"scale": "=drag.root.x"}}`, "", "root/props/scale", "drag driver"},
		{"param without default", "param id: string", `{"type": "spacer", "id": "root"}`, "", "script", "default"},
		{"bind with value", "state e = \"\"", `{"type": "textField", "id": "root", "props": {"bind": "e", "value": "x"}}`, "", "root/props/bind", "value"},
		{"bind not state", "let e = \"\"", `{"type": "textField", "id": "root", "props": {"bind": "e"}}`, "", "root/props/bind", "assign"},
		{"bind on text", "state e = \"\"", `{"type": "text", "id": "root", "props": {"text": "a", "bind": "e"}}`, "", "root/props/bind", "unknown prop"},
		{"timeline target", "", `{"type": "spacer", "id": "root"}`, `[{"name": "t", "tracks": [{"target": "nope", "prop": "opacity", "keys": [{"t": 0, "v": 0}]}]}]`, "timelines/0/tracks/0", "nope"},
		{"timeline prop", "", `{"type": "spacer", "id": "root"}`, `[{"name": "t", "tracks": [{"target": "root", "prop": "width", "keys": [{"t": 0, "v": 0}]}]}]`, "timelines/0/tracks/0", "width"},
		{"timeline dup track", "", `{"type": "spacer", "id": "root"}`, `[{"name": "t", "tracks": [{"target": "root", "prop": "opacity", "keys": [{"t": 0, "v": 0}]}, {"target": "root", "prop": "opacity", "keys": [{"t": 0, "v": 1}]}]}]`, "timelines/0/tracks/1", "duplicate"},
		{"timeline repeat", "", `{"type": "spacer", "id": "root"}`, `[{"name": "t", "repeat": 0, "tracks": [{"target": "root", "prop": "opacity", "keys": [{"t": 0, "v": 0}]}]}]`, "timelines/0", "repeat"},
		{"timeline key t", "", `{"type": "spacer", "id": "root"}`, `[{"name": "t", "tracks": [{"target": "root", "prop": "opacity", "keys": [{"t": -1, "v": 0}]}]}]`, "timelines/0/tracks/0/keys/0", "t"},
		{"timeline ease", "", `{"type": "spacer", "id": "root"}`, `[{"name": "t", "tracks": [{"target": "root", "prop": "opacity", "keys": [{"t": 0, "v": 0, "ease": "bounce"}]}]}]`, "timelines/0/tracks/0/keys/0", "ease"},
		{"timeline dup name", "", `{"type": "spacer", "id": "root"}`, `[{"name": "t", "tracks": []}, {"name": "t", "tracks": []}]`, "timelines/1", "duplicate"},
		{"timeline name", "", `{"type": "spacer", "id": "root"}`, `[{"name": "a b", "tracks": []}]`, "timelines/0", "name"},
		{"play unknown", "", `{"type": "shape", "id": "root", "on": {"tap": "play(nope)"}}`, "", "root/on/tap", "timeline"},
		{"transition NaN", "", `{"type": "spacer", "id": "root", "transition": {"in": "scale(NaN)"}}`, "", "root/transition/in", "finite"},
		{"number out of range", "", `{"type": "spacer", "id": "root", "props": {"opacity": 1e999}}`, "", "root/props/opacity", "out of range"},
		{"number out of range nested", "", `{"type": "spacer", "id": "root", "props": {"padding": {"top": -1e999}}}`, "", "root/props/padding", "out of range"},
		{"@ expression", "", `{"type": "spacer", "id": "root", "props": {"opacity": "@space.lg * 2 + 1"}}`, "", "root/props/opacity", "="},
		{"lazy list in a card", `state xs = ["a"]`, `{"type": "scroll", "id": "root", "props": {"lazy": true}, "children": [{"type": "stack", "id": "card", "children": [{"type": "text", "id": "row", "repeat": {"over": "xs"}, "props": {"text": "=item"}}]}]}`, "", "root/children/0/children/0/repeat", "lazy scroll 'root': repeat 'row' is nested in 'card'"},
		{"lazy list in a repeated section", `state xs = ["a"]`, `{"type": "scroll", "id": "root", "props": {"lazy": true}, "children": [{"type": "stack", "id": "sec", "repeat": {"over": "xs"}, "children": [{"type": "text", "id": "row", "repeat": {"over": "xs"}, "props": {"text": "=item"}}]}]}`, "", "root/children/0/children/0/repeat", "nested in 'sec'"},
		{"lazy list from a template", "", `{"type": "scroll", "id": "root", "props": {"lazy": true}, "children": [{"type": "Inner", "id": "inn", "props": {"tag": "a"}}]}`, "", "root/children/0", "repeat 'inn/t' is nested in 'inn'"},
	}
	for _, c := range cases {
		_, _, diags := Screen(contract(t), src(c.script, c.root, c.timelines), 1)
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

// Rows as direct children of a lazy scroll, repeats inside horizontal containers (carousels) and nested lists in a
// non-lazy scroll compile.
func TestLazyScrollListsOK(t *testing.T) {
	compileOK(t, src(`state xs = ["a"]`, `{"type": "stack", "id": "root", "children": [
	{"type": "scroll", "id": "list", "props": {"lazy": true}, "children": [
		{"type": "text", "id": "row", "repeat": {"over": "xs"}, "props": {"text": "=item"}},
		{"type": "stack", "id": "chips", "props": {"axis": "h"}, "children": [{"type": "text", "id": "chip", "repeat": {"over": "xs"}, "props": {"text": "=item"}}]},
		{"type": "stack", "id": "section", "children": [
			{"type": "text", "id": "title", "props": {"text": "Карусель"}},
			{"type": "scroll", "id": "rail", "props": {"axis": "h"}, "children": [
				{"type": "stack", "id": "card", "repeat": {"over": "xs"}, "children": [{"type": "text", "id": "cardText", "props": {"text": "=item"}}]}
			]}
		]}
	]},
	{"type": "scroll", "id": "eager", "props": {"lazy": false}, "children": [{"type": "stack", "id": "box", "children": [{"type": "text", "id": "inner", "repeat": {"over": "xs"}, "props": {"text": "=item"}}]}]}
	]}`, ""))
}

func TestDiagPosition(t *testing.T) {
	_, _, diags := Screen(contract(t), src("state a = 1\nstate b = nope", `{"type": "text", "id": "root", "props": {"text": "=a +\n  zzz"}}`, ""), 1)
	want := map[string][2]int{"script": {2, 11}, "root/props/text": {2, 3}}
	for _, d := range diags {
		if w, ok := want[d.Path]; ok && (d.Line != w[0] || d.Col != w[1]) {
			t.Errorf("%s at %d:%d, want %v (%s)", d.Path, d.Line, d.Col, w, d.Msg)
		}
		delete(want, d.Path)
	}
	if len(want) > 0 {
		t.Errorf("missing diags for %v: %v", want, diags)
	}
}

func TestLimits(t *testing.T) {
	deep := `{"type": "spacer", "id": "n33"}`
	for i := 32; i >= 1; i-- {
		deep = `{"type": "stack", "id": "n` + itoa(i) + `", "children": [` + deep + `]}`
	}
	var wide []string
	for i := 0; i < 2001; i++ {
		wide = append(wide, `{"type": "spacer", "id": "w`+itoa(i)+`"}`)
	}
	big := `{"type": "text", "id": "root", "props": {"text": "` + strings.Repeat("x", 600*1024) + `"}}`
	for name, root := range map[string]string{
		"depth": deep,
		"nodes": `{"type": "stack", "id": "root", "children": [` + strings.Join(wide, ",") + `]}`,
		"size":  big,
	} {
		_, _, diags := Screen(contract(t), src("", root, ""), 1)
		if len(diags) != 1 || !strings.Contains(diags[0].Msg, "limit") || diags[0].Path != "root" {
			t.Errorf("%s: diags %v", name, diags)
		}
	}
	ok := `{"type": "spacer", "id": "n32"}`
	for i := 31; i >= 1; i-- {
		ok = `{"type": "stack", "id": "n` + itoa(i) + `", "children": [` + ok + `]}`
	}
	compileOK(t, src("", ok, ""))
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

// shape.trim — the stroked part of a circle, as fractions of the circumference from 12 o'clock clockwise.
func TestShapeTrim(t *testing.T) {
	doc, _ := compileOK(t, src(`param to: number = 0.375`, `{"type": "stack", "id": "root", "children": [
	  {"type": "shape", "id": "lit", "props": {"kind": "circle", "stroke": "={color: @color.el16, width: 4}", "trim": {"from": 0, "to": 0.25}}},
	  {"type": "shape", "id": "expr", "props": {"kind": "circle", "trim": "={from: 0.1, to: to}"}}]}`, ""))
	if got := js(at(doc, "root/c/0/p/trim")); got != `["v",{"from":0,"to":0.25}]` {
		t.Errorf("literal trim: %s", got)
	}
	if got := js(at(doc, "root/c/1/p/trim")); got != `["rec",{"from":["v",0.1],"to":["p","to"]}]` {
		t.Errorf("expression trim: %s", got)
	}
	for _, bad := range []string{`"trim": 0.5`, `"trim": {"from": 0}`, `"trim": "={from: 0, to: \"x\"}"`} {
		_, _, diags := Screen(contract(t), src("", `{"type": "shape", "id": "root", "props": {`+bad+`}}`, ""), 1)
		if len(diags) == 0 || diags[0].Path != "root/props/trim" {
			t.Errorf("%s: diags %v, want one at root/props/trim", bad, diags)
		}
	}
}

func TestTransitionParse(t *testing.T) {
	cases := map[string]string{
		"fade":                         `[{"k":"fade"}]`,
		" fade+slide( top ) ":          `[{"k":"fade"},{"k":"slide","edge":"top"}]`,
		"scale(0.5) + offset(-1.5, 2)": `[{"k":"scale","from":0.5},{"k":"offset","x":-1.5,"y":2}]`,
	}
	for in, want := range cases {
		tr, err := parseTransition(in)
		if err != nil || js(tr) != want {
			t.Errorf("%q: %s %v, want %s", in, js(tr), err, want)
		}
	}
	for _, in := range []string{"", "fade +", "slide()", "scale(a)", "offset(1)", "fade(1)", "wipe", "scale(NaN)", "offset(Inf, 0)", "scale(-inf)"} {
		if _, err := parseTransition(in); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
}

func TestLoadContract(t *testing.T) {
	c, err := LoadContract(contractDir)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Tokens["color"]["el16"] || !c.Tokens["icon"]["close"] || !c.Tokens["motion"]["snappy"] {
		t.Error("tokens not loaded")
	}
	if p := c.Prims.Prims["text"]; p == nil || !p.Props["text"].Required || p.Children {
		t.Errorf("primitives: %+v", p)
	}
	if _, err := loadContract(contractDir, "testdata/bad_templates"); err == nil || !strings.Contains(err.Error(), "Broken") {
		t.Errorf("bad template must fail naming it: %v", err)
	}
}

// FuzzScreen: any source yields diagnostics or a valid document, never a panic.
func FuzzScreen(f *testing.F) {
	for _, s := range []string{
		`{"type": "stack", "id": "root", "children": [{"type": "Button", "id": "b", "props": {"title": "=str(n)"}, "on": {"tap": "n = n + 1"}}]}`,
		`{"type": "stack", "id": "root", "children": [{"type": "Card", "id": "c", "repeat": {"over": "[\"a\"]"}, "props": {"items": "=[item]"}, "children": [{"type": "Toggle", "id": "t"}]}]}`,
		`{"type": "Row", "id": "r", "props": {"code": "x"}, "on": {"pick": "n = 0"}}`,
		`{"type": "textField", "id": "root", "props": {"bind": "e"}, "drag": {"axis": "x"}, "on": {"dragEnd": "n = event.x"}}`,
		`{"type": "stack", "id": "root", "transition": {"in": "fade + slide(bottom) + scale(0.9)", "out": "offset(0, 20)"}, "when": "n > 0"}`,
		`{"type": "stack", "id": "root", "children": [{"type": "Form", "id": "f", "on": {"submit": "e = event.text"}}, {"type": "Hint", "id": "h", "props": {"text": "a", "show": false}}, {"type": "spacer", "id": "x", "when": "false"}]}`,
	} {
		f.Add(string(src("state n = 0\nstate e = \"\"", s, `[{"name": "t", "tracks": [{"target": "root", "prop": "opacity", "keys": [{"t": 0, "v": 0, "ease": "spring"}]}]}]`)))
	}
	c, err := loadContract(contractDir, "testdata/templates")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, source string) {
		doc, _, diags := Screen(c, []byte(source), 1)
		if len(diags) == 0 && !json.Valid(doc) {
			t.Fatalf("invalid document %s", doc)
		}
	})
}
