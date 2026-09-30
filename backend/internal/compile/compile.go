package compile

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"wishbone/backend/internal/dsl"
)

// Diag is a compile diagnostic (contract/api.md): Path uses source JSON keys; Line/Col are relative to the DSL
// fragment at Path (0 when the error is not inside a fragment).
type Diag struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Msg  string `json:"message"`
}

func (d Diag) String() string { return fmt.Sprintf("%s:%d:%d: %s", d.Path, d.Line, d.Col, d.Msg) }

type DataDecl = dsl.DataSource

// Meta is what the server stores next to a revision.
type Meta struct {
	RuntimeMin string
	Ops        []string // operations used by `call` (the /v1/call allowlist)
	Data       []DataDecl
	Links      []Link // without duplicates, sorted by (To, Kind, Via)
}

// Link is a screen transition: `navigate`/`present` with a literal target (["v", id]); dynamic targets are skipped.
type Link struct {
	To   string `json:"to"`
	Kind string `json:"kind"` // "navigate" | "present"
	Via  string `json:"via"`  // "node:<id>" | "handler:<name>"
}

// Document is the wire document (spec §3.8).
type Document struct {
	Format     int                        `json:"format"`
	RuntimeMin string                     `json:"runtimeMin"`
	Screen     string                     `json:"screen"`
	Revision   int                        `json:"revision"`
	Params     map[string]json.RawMessage `json:"params"`
	State      []json.RawMessage          `json:"state"`
	Computed   map[string]json.RawMessage `json:"computed"`
	Fns        map[string]dsl.Body        `json:"fns"`
	Handlers   map[string]dsl.Body        `json:"handlers"`
	Data       []dsl.DataSource           `json:"data"`
	Timelines  map[string]*Timeline       `json:"timelines"`
	Root       *Node                      `json:"root"`
}

type Node struct {
	T     string                     `json:"t"`
	ID    string                     `json:"id"`
	P     map[string]json.RawMessage `json:"p,omitempty"`
	C     []*Node                    `json:"c,omitempty"`
	When  json.RawMessage            `json:"when,omitempty"`
	Rep   *Repeat                    `json:"rep,omitempty"`
	On    map[string]json.RawMessage `json:"on,omitempty"`
	Anim  map[string]json.RawMessage `json:"anim,omitempty"`
	Tr    *Transitions               `json:"tr,omitempty"`
	Match json.RawMessage            `json:"match,omitempty"`
	Drag  json.RawMessage            `json:"drag,omitempty"`
	Inst  *Inst                      `json:"inst,omitempty"`
	A11y  *A11y                      `json:"a11y,omitempty"`
}

type Repeat struct {
	Over  json.RawMessage `json:"over"`
	As    string          `json:"as"`
	Index string          `json:"index"`
	Key   json.RawMessage `json:"key,omitempty"`
}

type Transitions struct {
	In  []Trans `json:"in,omitempty"`
	Out []Trans `json:"out,omitempty"`
}

type Inst struct {
	State map[string]json.RawMessage `json:"state"`
}

type A11y struct {
	Label  json.RawMessage `json:"label,omitempty"`
	Role   string          `json:"role,omitempty"`
	Hidden json.RawMessage `json:"hidden,omitempty"`
}

const (
	maxSize  = 512 * 1024
	maxDepth = 32
	maxNodes = 2000
)

// Screen compiles a screen source into a wire document.
func Screen(c *Contract, src []byte, rev int) (json.RawMessage, Meta, []Diag) {
	u, doc := compileScreen(c, src, rev)
	if doc == nil {
		return nil, Meta{}, u.diags
	}
	b := encode(doc)
	if len(b) > maxSize {
		return nil, Meta{}, []Diag{{Path: "root", Msg: fmt.Sprintf("limit: document is %d KB, the maximum is %d KB", len(b)/1024, maxSize/1024)}}
	}
	return b, meta(doc), nil
}

func compileScreen(c *Contract, src []byte, rev int) (*unit, *Document) {
	u := newUnit(c, "")
	s, root, decls := u.compile(src)
	if len(u.diags) > 0 {
		return u, nil
	}
	for name, d := range u.fns {
		decls.Fns[name] = d
	}
	doc := &Document{Format: 1, Screen: s.ID, Revision: rev, Params: decls.Params,
		State: decls.State, Computed: decls.Computed, Fns: decls.Fns, Handlers: decls.Handlers, Data: decls.Data,
		Timelines: u.tls, Root: root}
	prune(doc)
	doc.RuntimeMin = runtimeMin(doc.Root)
	u.lazyLists(root)
	nodes, depth := count(root, 1)
	if nodes > maxNodes {
		u.errf("root", "limit: %d nodes after template expansion, the maximum is %d", nodes, maxNodes)
	}
	if depth > maxDepth {
		u.errf("root", "limit: tree depth %d after template expansion, the maximum is %d", depth, maxDepth)
	}
	if len(u.diags) > 0 {
		return u, nil
	}
	return u, doc
}

// prune removes nodes whose `when` is the literal false after template expansion (with their subtrees), drops
// literal-true `when`, then timeline tracks on removed nodes, timelines left without tracks and play/stop of those.
func prune(doc *Document) {
	ids := map[string]bool{}
	var walk func(n *Node)
	walk = func(n *Node) {
		ids[n.ID] = true
		kept := n.C[:0]
		for _, c := range n.C {
			switch string(c.When) {
			case `["v",false]`:
				continue
			case `["v",true]`:
				c.When = nil
			}
			walk(c)
			kept = append(kept, c)
		}
		n.C = kept
	}
	if string(doc.Root.When) == `["v",true]` {
		doc.Root.When = nil
	}
	walk(doc.Root)
	dropped := map[string]bool{}
	for name, tl := range doc.Timelines {
		kept := tl.Tracks[:0]
		for _, tr := range tl.Tracks {
			if ids[tr.Target] {
				kept = append(kept, tr)
			}
		}
		if tl.Tracks = kept; len(kept) == 0 {
			delete(doc.Timelines, name)
			dropped[name] = true
		}
	}
	if len(dropped) == 0 {
		return
	}
	var drop func(ss []any) []any
	drop = func(ss []any) []any {
		out := []any{}
		for _, s := range ss {
			a := asList(s)
			switch a[0] {
			case "do":
				if (a[1] == "play" || a[1] == "stop") && dropped[asList(asList(a[2])[0])[1].(string)] {
					continue
				}
			case "if":
				a = []any{"if", a[1], drop(asList(a[2])), drop(asList(a[3]))}
			case "anim":
				a = []any{"anim", a[1], drop(asList(a[2]))}
			}
			out = append(out, a)
		}
		return out
	}
	for name, h := range doc.Handlers {
		h.Body = encode(drop(asList(decode(h.Body))))
		doc.Handlers[name] = h
	}
	var nodes func(n *Node)
	nodes = func(n *Node) {
		for ev, body := range n.On {
			n.On[ev] = encode(drop(asList(decode(body))))
		}
		for _, c := range n.C {
			nodes(c)
		}
	}
	nodes(doc.Root)
}

// lazyLists rejects a repeat nested inside a child of a vertical lazy scroll (decisions/renderers.md): the lazy
// container virtualizes only its direct children, and one child taller than the screen livelocks SwiftUI's
// LazyVStack (iOS 26) and stops Compose from re-firing a paging `appear`. It runs on the expanded, pruned tree —
// templates carry repeats too. Horizontal stacks and scrolls are not entered: a carousel can't outgrow the screen
// height. A repeated child is checked inside as well (a repeated section with a nested list is the same problem).
// A non-literal `lazy` counts as lazy and a non-literal `axis` as vertical.
func (u *unit) lazyLists(n *Node) {
	horizontal := func(m *Node) bool { return (m.T == "stack" || m.T == "scroll") && string(m.P["axis"]) == `["v","h"]` }
	if n.T == "scroll" && n.P["lazy"] != nil && string(n.P["lazy"]) != `["v",false]` && !horizontal(n) {
		for _, c := range n.C {
			var nested func(m *Node)
			nested = func(m *Node) {
				if horizontal(m) {
					return
				}
				for _, k := range m.C {
					if k.Rep == nil {
						nested(k)
						continue
					}
					u.errf(u.repeatPath(k.ID), "lazy scroll '%s': repeat '%s' is nested in '%s' — make the rows direct children of the scroll (a child taller than the screen hangs iOS)", n.ID, k.ID, c.ID)
				}
			}
			nested(c)
		}
	}
	for _, c := range n.C {
		u.lazyLists(c)
	}
}

// repeatPath is the source path of an expanded node's repeat: `…/repeat` of a screen node, else the node of the
// template instance it was expanded from (expanded ids are `<instance id>/<template id>`).
func (u *unit) repeatPath(id string) string {
	first, rest, _ := strings.Cut(id, "/")
	s := u.ids[first]
	switch {
	case s == nil:
		return "root"
	case rest == "" && s.Repeat != nil:
		return s.path + "/repeat"
	default:
		return s.path
	}
}

// since is the runtime that first understands a feature, keyed `<primitive>.<prop>`; anything unlisted is 1.0.0.
// ponytail: node props only; key expression / statement forms here too when the first such 1.x feature lands.
var since = map[string][3]int{"shape.trim": {1, 1, 0}}

// runtimeMin is the oldest runtime that understands the document: the latest `since` over the features left in the
// expanded, pruned tree.
func runtimeMin(root *Node) string {
	v := [3]int{1, 0, 0}
	var walk func(n *Node)
	walk = func(n *Node) {
		for k := range n.P {
			if s, ok := since[n.T+"."+k]; ok && slices.Compare(s[:], v[:]) > 0 {
				v = s
			}
		}
		for _, c := range n.C {
			walk(c)
		}
	}
	walk(root)
	return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
}

func count(n *Node, depth int) (nodes, maxd int) {
	nodes, maxd = 1, depth
	for _, c := range n.C {
		k, d := count(c, depth+1)
		nodes += k
		maxd = max(maxd, d)
	}
	return nodes, maxd
}

func meta(doc *Document) Meta {
	ops := map[string]bool{}
	links := map[Link]bool{}
	var walk func(ss []any, via string)
	walk = func(ss []any, via string) {
		for _, s := range ss {
			a, _ := s.([]any)
			switch {
			case len(a) > 1 && a[0] == "call":
				ops[a[1].(string)] = true
			case len(a) > 2 && a[0] == "do" && (a[1] == "navigate" || a[1] == "present"):
				if to := asList(asList(a[2])[0]); to[0] == "v" {
					links[Link{To: to[1].(string), Kind: a[1].(string), Via: via}] = true
				}
			case len(a) > 3 && a[0] == "if":
				walk(asList(a[2]), via)
				walk(asList(a[3]), via)
			case len(a) > 2 && a[0] == "anim":
				walk(asList(a[2]), via)
			}
		}
	}
	for name, h := range doc.Handlers {
		walk(asList(decode(h.Body)), "handler:"+name)
	}
	var nodes func(n *Node)
	nodes = func(n *Node) {
		for _, body := range n.On {
			walk(asList(decode(body)), "node:"+n.ID)
		}
		for _, c := range n.C {
			nodes(c)
		}
	}
	nodes(doc.Root)
	m := Meta{RuntimeMin: doc.RuntimeMin, Ops: []string{}, Data: doc.Data, Links: []Link{}}
	for op := range ops {
		m.Ops = append(m.Ops, op)
	}
	sort.Strings(m.Ops)
	for l := range links {
		m.Links = append(m.Links, l)
	}
	slices.SortFunc(m.Links, func(a, b Link) int {
		return cmp.Or(strings.Compare(a.To, b.To), strings.Compare(a.Kind, b.Kind), strings.Compare(a.Via, b.Via))
	})
	return m
}

// --- source ---

type source struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Script    string            `json:"script"`
	Timelines []json.RawMessage `json:"timelines"`
	Root      json.RawMessage   `json:"root"`
}

type srcNode struct {
	Type       string                     `json:"type"`
	ID         string                     `json:"id"`
	Props      map[string]json.RawMessage `json:"props"`
	Children   []json.RawMessage          `json:"children"`
	On         map[string]string          `json:"on"`
	When       *string                    `json:"when"`
	Repeat     *srcRepeat                 `json:"repeat"`
	Animate    map[string]string          `json:"animate"`
	Transition *srcTransition             `json:"transition"`
	MatchID    *string                    `json:"matchId"`
	Drag       json.RawMessage            `json:"drag"`
	A11y       *srcA11y                   `json:"a11y"`

	path string
	kids []*srcNode
}

type srcRepeat struct {
	Over  string  `json:"over"`
	As    *string `json:"as"`
	Index *string `json:"index"`
	Key   *string `json:"key"`
}

type srcTransition struct {
	In  *string `json:"in"`
	Out *string `json:"out"`
}

type srcA11y struct {
	Label  *string         `json:"label"`
	Role   *string         `json:"role"`
	Hidden json.RawMessage `json:"hidden"`
}

// unit is one compilation: a screen, or a template compiled standalone.
type unit struct {
	c     *Contract
	tmpl  string // template name; "" for a screen
	diags []Diag
	prog  *dsl.Program
	ids   map[string]*srcNode
	fns   map[string]dsl.Body // fns hoisted from expanded templates
	tls   map[string]*Timeline
	slots int
	seq   int // hygiene counter for template expansion
}

func newUnit(c *Contract, tmpl string) *unit {
	return &unit{c: c, tmpl: tmpl, ids: map[string]*srcNode{}, fns: map[string]dsl.Body{}, tls: map[string]*Timeline{}}
}

func (u *unit) errf(path string, format string, a ...any) {
	u.diags = append(u.diags, Diag{Path: path, Msg: fmt.Sprintf(format, a...)})
}

func (u *unit) dslDiags(path string, ds []dsl.Diag) {
	for _, d := range ds {
		u.diags = append(u.diags, Diag{Path: path, Line: d.Line, Col: d.Col, Msg: d.Msg})
	}
}

func strict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "json: "))
	}
	return nil
}

// compile runs the shared pipeline: source → script check → timelines → node tree → lowered declarations.
// decls is nil when the script has diagnostics.
func (u *unit) compile(src []byte) (*source, *Node, *dsl.Decls) {
	var s source
	if err := strict(src, &s); err != nil {
		u.errf("", "invalid source: %v", err)
		return &s, nil, nil
	}
	script, d := dsl.ParseScript(s.Script)
	u.dslDiags("script", d)
	var tree *srcNode
	if s.Root == nil {
		u.errf("root", "missing root")
	} else {
		if tree = u.decodeTree(s.Root, "root"); tree != nil {
			u.collectIDs(tree)
		}
	}
	srcTLs := make([]*srcTimeline, len(s.Timelines))
	names := map[string]bool{}
	for i, raw := range s.Timelines {
		path := fmt.Sprintf("timelines/%d", i)
		t := &srcTimeline{}
		if err := strict(raw, t); err != nil {
			u.errf(path, "invalid timeline: %v", err)
			continue
		}
		switch {
		case !isIdent(t.Name):
			u.errf(path, "timeline name %q must be an identifier", t.Name)
		case names[t.Name]:
			u.errf(path, "duplicate timeline '%s'", t.Name)
		default:
			names[t.Name] = true
			srcTLs[i] = t
		}
	}
	env := &dsl.Env{Stdlib: u.c.Stdlib, Ops: u.c.Ops, Tokens: u.c.Tokens, Timelines: names, Drivers: u.driver, Template: u.tmpl != ""}
	prog, d := dsl.Check(script, env)
	u.dslDiags("script", d)
	scriptOK := len(u.diags) == 0
	u.prog = prog
	u.checkDecls(script)
	for i, t := range srcTLs {
		if t != nil {
			u.tls[t.Name] = u.timeline(t, fmt.Sprintf("timelines/%d", i), func(id string) bool { return u.ids[id] != nil })
		}
	}
	var root *Node
	if tree != nil {
		root = u.node(tree, nil)
	}
	if !scriptOK {
		return &s, root, nil
	}
	return &s, root, prog.Lower()
}

func (u *unit) checkDecls(s *dsl.Script) {
	for _, d := range s.Decls {
		switch d := d.(type) {
		case *dsl.ParamDecl:
			if u.tmpl == "" && d.Default == nil {
				u.diags = append(u.diags, Diag{"script", d.P.Line, d.P.Col, fmt.Sprintf("screen param '%s' needs a default value", d.Name)})
			}
		case *dsl.DataDecl:
			if u.tmpl != "" {
				u.diags = append(u.diags, Diag{"script", d.P.Line, d.P.Col, "data sources are not allowed in templates; pass values as params"})
			}
		}
	}
}

func (u *unit) decodeTree(raw json.RawMessage, path string) *srcNode {
	n := &srcNode{}
	if err := strict(raw, n); err != nil {
		u.errf(path, "invalid node: %v", err)
		return nil
	}
	n.path = path
	for i, c := range n.Children {
		if k := u.decodeTree(c, fmt.Sprintf("%s/children/%d", path, i)); k != nil {
			n.kids = append(n.kids, k)
		}
	}
	return n
}

func (u *unit) collectIDs(n *srcNode) {
	if n.Type != "slot" {
		switch {
		case n.ID == "":
			u.errf(n.path, "missing id")
		case strings.Contains(n.ID, "/"):
			u.errf(n.path+"/id", "id %q must not contain '/' (reserved for template expansion)", n.ID)
		case u.ids[n.ID] != nil:
			u.errf(n.path+"/id", "duplicate id '%s'", n.ID)
		default:
			u.ids[n.ID] = n
		}
	}
	for _, k := range n.kids {
		u.collectIDs(k)
	}
}

// driver tells the checker whether scroll.<id> / drag.<id> names a suitable node.
func (u *unit) driver(kind, id string) bool {
	n := u.ids[id]
	if n == nil {
		return false
	}
	typ, drag := n.Type, n.Drag != nil
	if t, _ := u.c.template(n.Type); t != nil {
		typ, drag = t.Root.T, drag || t.Root.Drag != nil
	}
	if kind == "scroll" {
		return typ == "scroll"
	}
	return drag
}

// --- nodes ---

func (u *unit) node(n *srcNode, locals map[string]*dsl.Type) *Node {
	switch {
	case n.Type == "slot":
		return u.slot(n)
	case isTemplateType(n.Type):
		return u.instance(n, locals)
	}
	prim := u.c.Prims.Prims[n.Type]
	if prim == nil {
		u.errf(n.path, "unknown type '%s'", n.Type)
		return nil
	}
	w := &Node{T: n.Type, ID: n.ID}
	locals = u.repeat(n, w, locals)
	self := hasKey(n.On, "tap") || hasKey(n.On, "longPress")
	sc := &dsl.Scope{Locals: locals, Self: self}
	u.common(n, w, locals)

	var bindOn []json.RawMessage
	for _, k := range sortedKeys(n.Props) {
		path := n.path + "/props/" + k
		if k == "bind" && prim.Sugar["bind"] != "" {
			bindOn = u.bind(n, w, prim, sc, path)
			continue
		}
		p := prim.Props[k]
		if p == nil {
			p = u.c.Prims.Common[k]
		}
		if p == nil {
			u.errf(path, "unknown prop '%s' for %s", k, n.Type)
			continue
		}
		u.setProp(w, k, u.value(n.Props[k], p.Type, sc, path))
	}
	for _, k := range sortedKeys(prim.Props) {
		if prim.Props[k].Required && n.Props[k] == nil {
			u.errf(n.path, "missing required prop '%s' for %s", k, n.Type)
		}
	}
	// A literal non-circle kind (rect is the default) never draws a trim; a non-literal kind is left to the renderer.
	if kind := string(w.P["kind"]); n.Type == "shape" && w.P["trim"] != nil && (n.Props["kind"] == nil || (strings.HasPrefix(kind, `["v",`) && kind != `["v","circle"]`)) {
		u.errf(n.path+"/props/trim", `trim draws only on a circle; set kind to "circle"`)
	}

	events := u.eventTypes(n, prim.Events)
	for _, ev := range sortedKeys(n.On) {
		path := n.path + "/on/" + ev
		pt, ok := events[ev]
		if !ok {
			u.errf(path, "unknown event '%s' for %s", ev, n.Type)
			continue
		}
		body := u.stmts(n.On[ev], &dsl.Scope{Locals: locals, Event: pt, Self: self}, path)
		if ev == "change" {
			body = append(bindOn, body...)
		}
		u.setOn(w, ev, body)
	}
	if bindOn != nil && !hasKey(n.On, "change") {
		u.setOn(w, "change", bindOn)
	}

	if len(n.kids) > 0 && !prim.Children {
		u.errf(n.path, "%s has no children", n.Type)
	}
	w.C = u.children(n, locals)
	return w
}

func (u *unit) children(n *srcNode, locals map[string]*dsl.Type) []*Node {
	var out []*Node
	for _, k := range n.kids {
		if c := u.node(k, locals); c != nil {
			out = append(out, c)
		}
	}
	return out
}

func (u *unit) eventTypes(n *srcNode, own map[string]*dsl.Type) map[string]*dsl.Type {
	events := map[string]*dsl.Type{}
	for k, t := range u.c.Prims.Events {
		events[k] = t
	}
	for k, t := range own {
		events[k] = t
	}
	if n.Drag != nil {
		for k, t := range u.c.Prims.DragEvents {
			events[k] = t
		}
	}
	return events
}

func (u *unit) bind(n *srcNode, w *Node, prim *Primitive, sc *dsl.Scope, path string) []json.RawMessage {
	var target string
	switch {
	case json.Unmarshal(n.Props["bind"], &target) != nil || target == "":
		u.errf(path, `bind must name the state to bind, e.g. "email"`)
	case n.Props["value"] != nil:
		u.errf(path, "bind and value cannot both be set (bind already sets value)")
	default:
		v, _ := u.expr(target, dsl.TString, sc, path)
		u.setProp(w, "value", v)
		return u.stmts(target+" = event.value", &dsl.Scope{Locals: sc.Locals, Event: prim.Events["change"], Self: sc.Self}, path)
	}
	return nil
}

func (u *unit) setProp(w *Node, k string, v json.RawMessage) {
	if v == nil {
		return
	}
	if w.P == nil {
		w.P = map[string]json.RawMessage{}
	}
	w.P[k] = v
}

func (u *unit) setOn(w *Node, ev string, body []json.RawMessage) {
	if w.On == nil {
		w.On = map[string]json.RawMessage{}
	}
	if body == nil {
		body = []json.RawMessage{}
	}
	w.On[ev] = encode(body)
}

func (u *unit) repeat(n *srcNode, w *Node, locals map[string]*dsl.Type) map[string]*dsl.Type {
	r := n.Repeat
	if r == nil {
		return locals
	}
	path := n.path + "/repeat"
	as, index := "item", "index"
	if r.As != nil {
		as = *r.As
	}
	if r.Index != nil {
		index = *r.Index
	}
	if !isIdent(as) || !isIdent(index) || as == index {
		u.errf(path, "repeat as/index must be two different identifiers")
	}
	over, t := u.expr(r.Over, nil, &dsl.Scope{Locals: locals}, path+"/over")
	var elem *dsl.Type
	if t != nil {
		if t.Kind == dsl.KOpt {
			t = t.Elem
		}
		if t.Kind == dsl.KList {
			elem = t.Elem
		} else {
			u.errf(path+"/over", "repeat.over must be a list, got %s", t)
		}
	}
	inner := map[string]*dsl.Type{as: elem, index: dsl.TNumber}
	for k, v := range locals {
		if _, shadowed := inner[k]; !shadowed {
			inner[k] = v
		}
	}
	w.Rep = &Repeat{Over: over, As: as, Index: index}
	if r.Key != nil {
		w.Rep.Key, _ = u.expr(*r.Key, dsl.UnionOf(dsl.TString, dsl.TNumber), &dsl.Scope{Locals: inner}, path+"/key")
	}
	return inner
}

// common compiles the fields every node (and every template instance) accepts, except repeat, props and on.
func (u *unit) common(n *srcNode, w *Node, locals map[string]*dsl.Type) {
	sc := &dsl.Scope{Locals: locals}
	if n.When != nil {
		w.When, _ = u.expr(*n.When, dsl.TBool, sc, n.path+"/when")
	}
	for _, k := range sortedKeys(n.Animate) {
		path := n.path + "/animate/" + k
		if k != "*" && !u.c.Prims.Animatable[k] {
			u.errf(path, "'%s' is not animatable (primitives.json animatable, or \"*\")", k)
			continue
		}
		if v, _ := u.expr(n.Animate[k], dsl.TMotion, sc, path); v != nil {
			if w.Anim == nil {
				w.Anim = map[string]json.RawMessage{}
			}
			w.Anim[k] = v
		}
	}
	if tr := n.Transition; tr != nil {
		w.Tr = &Transitions{}
		for _, x := range []struct {
			src *string
			dst *[]Trans
			key string
		}{{tr.In, &w.Tr.In, "in"}, {tr.Out, &w.Tr.Out, "out"}} {
			if x.src == nil {
				continue
			}
			t, err := parseTransition(*x.src)
			if err != nil {
				u.errf(n.path+"/transition/"+x.key, "%v", err)
			}
			*x.dst = t
		}
	}
	if n.MatchID != nil {
		w.Match, _ = u.expr(*n.MatchID, dsl.TString, sc, n.path+"/matchId")
	}
	if n.Drag != nil {
		w.Drag = u.drag(n.Drag, n.path+"/drag")
	}
	if a := n.A11y; a != nil {
		w.A11y = &A11y{}
		if a.Label != nil {
			w.A11y.Label, _ = u.expr(*a.Label, dsl.TString, sc, n.path+"/a11y/label")
		}
		if a.Role != nil {
			switch *a.Role {
			case "button", "header", "image", "none":
				w.A11y.Role = *a.Role
			default:
				u.errf(n.path+"/a11y/role", "a11y role must be button, header, image or none")
			}
		}
		if a.Hidden != nil {
			w.A11y.Hidden = u.value(a.Hidden, dsl.TBool, sc, n.path+"/a11y/hidden")
		}
	}
}

// --- DSL fragments ---

// expr compiles a DSL expression fragment; nil wire on error.
func (u *unit) expr(src string, want *dsl.Type, sc *dsl.Scope, path string) (json.RawMessage, *dsl.Type) {
	e, d := dsl.ParseExpr(src)
	return u.checked(e, d, want, sc, path)
}

// checked type-checks and lowers a parsed fragment (d = its parse diagnostics).
func (u *unit) checked(e dsl.Expr, d []dsl.Diag, want *dsl.Type, sc *dsl.Scope, path string) (json.RawMessage, *dsl.Type) {
	if e != nil && len(d) == 0 {
		d = u.prog.CheckExpr(e, sc, want)
	}
	if len(d) > 0 {
		u.dslDiags(path, d)
		return nil, nil
	}
	return dsl.LowerExpr(e), e.Type()
}

func (u *unit) stmts(src string, sc *dsl.Scope, path string) []json.RawMessage {
	ss, d := dsl.ParseStmts(src)
	if len(d) == 0 {
		d = u.prog.CheckStmts(ss, sc)
	}
	if len(d) > 0 {
		u.dslDiags(path, d)
		return nil
	}
	var out []json.RawMessage
	if err := json.Unmarshal(dsl.LowerStmts(ss), &out); err != nil {
		panic(err)
	}
	return out
}

// --- helpers ---

func encode(v any) json.RawMessage {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimRight(b.Bytes(), "\n")
}

func decode(raw json.RawMessage) any {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		panic(err)
	}
	return v
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func hasKey[V any](m map[string]V, k string) bool {
	_, ok := m[k]
	return ok
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
