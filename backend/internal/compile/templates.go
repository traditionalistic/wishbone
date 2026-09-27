package compile

import (
	"encoding/json"
	"fmt"
	"strings"

	"wishbone/backend/internal/dsl"
)

// Template is a template compiled standalone (spec §2.3). Its wire tree still holds placeholders: params
// ["p", n], computed ["c", n], handler runs ["run", h, …] and ["emit", e]. Instance state and fns are already
// template-qualified ("Toggle.on", "Button.double"); ids, drivers and timeline names are relative to the template.
type Template struct {
	Name      string
	Params    []TemplateParam
	Events    map[string]*dsl.Type // declared events → payload record (nil = no payload)
	Slot      bool
	Root      *Node
	Computed  map[string]json.RawMessage
	Handlers  map[string]dsl.Body
	Fns       map[string]dsl.Body  // qualified, including nested templates'
	Timelines map[string]*Timeline // including nested templates' (named <instanceId>/<name>)
}

type TemplateParam struct {
	Name    string
	Type    *dsl.Type
	Default json.RawMessage // wire expression; nil = required at every instance
}

// template returns a compiled template, compiling it on first use while the contract loads (nil = unknown).
func (c *Contract) template(name string) (*Template, error) {
	if t := c.Templates[name]; t != nil {
		return t, nil
	}
	src, ok := c.tsrc[name]
	if !ok {
		return nil, nil
	}
	if c.tmark[name] == 1 {
		i := 0
		for c.tstk[i] != name {
			i++
		}
		return nil, fmt.Errorf("recursive template inclusion: %s", strings.Join(append(append([]string{}, c.tstk[i:]...), name), " → "))
	}
	c.tmark[name] = 1
	c.tstk = append(c.tstk, name)
	t, diags := compileTemplate(c, name, src)
	c.tstk = c.tstk[:len(c.tstk)-1]
	c.tmark[name] = 2
	if len(diags) > 0 {
		msgs := make([]string, len(diags))
		for i, d := range diags {
			msgs[i] = d.String()
		}
		return nil, fmt.Errorf("template %s: %s", name, strings.Join(msgs, "; "))
	}
	c.Templates[name] = t
	return t, nil
}

func compileTemplate(c *Contract, name string, src []byte) (*Template, []Diag) {
	u := newUnit(c, name)
	s, root, decls := u.compile(src)
	if s.ID != "" && s.ID != name {
		u.errf("id", "template id %q must match its file name %q", s.ID, name)
	}
	if len(u.diags) > 0 {
		return nil, u.diags
	}
	q := &rewriter{qualify: name}
	t := &Template{Name: name, Events: map[string]*dsl.Type{}, Slot: u.slots > 0, Computed: map[string]json.RawMessage{},
		Handlers: map[string]dsl.Body{}, Fns: map[string]dsl.Body{}, Timelines: u.tls}
	for _, d := range u.prog.Script.Decls {
		switch d := d.(type) {
		case *dsl.ParamDecl:
			tp := TemplateParam{Name: d.Name, Type: d.T}
			if d.Default != nil {
				tp.Default = decls.Params[d.Name]
			}
			t.Params = append(t.Params, tp)
		case *dsl.EventDecl:
			t.Events[d.Name] = d.Payload
		}
	}
	for k, v := range decls.Computed {
		t.Computed[k] = q.exprRaw(v)
	}
	for k, h := range decls.Handlers {
		t.Handlers[k] = dsl.Body{Params: h.Params, Body: q.stmtsRaw(h.Body)}
	}
	for k, f := range decls.Fns {
		t.Fns[q.q(k)] = dsl.Body{Params: f.Params, Body: q.exprRaw(f.Body)}
	}
	for k, f := range u.fns {
		t.Fns[k] = f
	}
	for _, st := range decls.State {
		var pair []json.RawMessage
		if err := json.Unmarshal(st, &pair); err != nil {
			panic(err)
		}
		var key string
		json.Unmarshal(pair[0], &key)
		if root.Inst == nil {
			root.Inst = &Inst{State: map[string]json.RawMessage{}}
		}
		root.Inst.State[key] = pair[1]
	}
	t.Root = q.node(root)[0]
	return t, nil
}

func (u *unit) slot(n *srcNode) *Node {
	switch {
	case u.tmpl == "":
		u.errf(n.path, "slot is only allowed in templates")
	case n.path == "root":
		u.errf(n.path, "a slot cannot be the template root")
	case n.ID != "" || n.Props != nil || n.Children != nil || n.On != nil || n.When != nil || n.Repeat != nil ||
		n.Animate != nil || n.Transition != nil || n.MatchID != nil || n.Drag != nil || n.A11y != nil:
		u.errf(n.path, `a slot is just {"type": "slot"}`)
	default:
		if u.slots++; u.slots > 1 {
			u.errf(n.path, "a template has at most one slot")
		}
		return &Node{T: "slot"}
	}
	return nil
}

// instance expands a template instance in place. Args, instance events and children are compiled in the
// instance's (outer) environment; the template body is rewritten around them.
func (u *unit) instance(n *srcNode, locals map[string]*dsl.Type) *Node {
	t, err := u.c.template(n.Type)
	if err != nil {
		u.errf(n.path, "%v", err)
		return nil
	}
	if t == nil {
		u.errf(n.path, "unknown template '%s'", n.Type)
		return nil
	}
	before := len(u.diags)
	w := &Node{} // instance-level fields, moved onto the expanded root
	locals = u.repeat(n, w, locals)
	if w.Rep != nil && t.Root.Rep != nil {
		u.errf(n.path+"/repeat", "the root of template %s already repeats; wrap the instance to repeat it", t.Name)
	}
	u.common(n, w, locals)
	sc := &dsl.Scope{Locals: locals}
	// modifiers land on the expanded root, so self.pressed works there when the root is pressable (ruling);
	// params are substituted into arbitrary inner nodes and never see self
	pressable := func(ev string) bool { return hasKey(t.Root.On, ev) || hasKey(n.On, ev) && !hasKey(t.Events, ev) }
	modSc := &dsl.Scope{Locals: locals, Self: pressable("tap") || pressable("longPress")}

	params := map[string]json.RawMessage{}
	isParam := map[string]bool{}
	for _, p := range t.Params {
		isParam[p.Name] = true
		switch raw, ok := n.Props[p.Name]; {
		case ok:
			params[p.Name] = u.value(raw, p.Type, sc, n.path+"/props/"+p.Name)
		case p.Default != nil:
			params[p.Name] = p.Default
		default:
			u.errf(n.path, "missing param '%s' of template %s", p.Name, t.Name)
		}
	}
	for _, k := range sortedKeys(n.Props) {
		if isParam[k] {
			continue
		}
		if p := u.c.Prims.Common[k]; p != nil {
			u.setProp(w, k, u.value(n.Props[k], p.Type, modSc, n.path+"/props/"+k))
		} else {
			u.errf(n.path+"/props/"+k, "unknown prop '%s' for template %s (params: %s)", k, t.Name, paramNames(t))
		}
	}

	emits := map[string][]json.RawMessage{}
	rootEvents := u.eventTypes(&srcNode{Drag: n.Drag}, u.c.Prims.Prims[t.Root.T].Events)
	if t.Root.Drag != nil {
		for k, pt := range u.c.Prims.DragEvents {
			rootEvents[k] = pt
		}
	}
	for _, ev := range sortedKeys(n.On) {
		path := n.path + "/on/" + ev
		switch {
		case hasKey(t.Events, ev): // `event` is the declared payload record, if any
			esc := &dsl.Scope{Locals: locals, Event: t.Events[ev]}
			if esc.Event == nil {
				esc.NoPayload = ev
			}
			emits[ev] = u.stmts(n.On[ev], esc, path)
		case rootEvents[ev] != nil:
			u.setOn(w, ev, u.stmts(n.On[ev], &dsl.Scope{Locals: locals, Event: rootEvents[ev]}, path))
		default:
			u.errf(path, "unknown event '%s' for template %s", ev, t.Name)
		}
	}
	kids := u.children(n, locals)
	if len(n.kids) > 0 && !t.Slot {
		u.errf(n.path, "template %s has no slot for children", t.Name)
	}

	if len(u.diags) > before { // expansion needs every argument compiled
		return nil
	}
	id := func(x string) string {
		if x == t.Root.ID {
			return n.ID
		}
		return n.ID + "/" + x
	}
	r := &rewriter{expand: true, pfx: u.fresh(t.Name), params: params, computed: t.Computed, handlers: t.Handlers,
		emits: emits, slot: kids, id: id, tl: func(name string) string { return n.ID + "/" + name }, fresh: u.fresh, tmpl: t.Name}
	root := r.node(t.Root)[0]
	mergeInstance(root, w)
	for name, f := range t.Fns {
		u.fns[name] = f
	}
	for name, tl := range t.Timelines {
		cp := *tl
		cp.Tracks = make([]Track, len(tl.Tracks))
		for i, tr := range tl.Tracks {
			tr.Target = id(tr.Target)
			cp.Tracks[i] = tr
		}
		u.tls[n.ID+"/"+name] = &cp
	}
	return root
}

// fresh returns a unique hygiene prefix for one expansion or one inlined handler.
func (u *unit) fresh(tmpl string) string {
	u.seq++
	return fmt.Sprintf("%s#%d.", tmpl, u.seq)
}

func paramNames(t *Template) string {
	var ns []string
	for _, p := range t.Params {
		ns = append(ns, p.Name)
	}
	if ns == nil {
		return "none"
	}
	return strings.Join(ns, ", ")
}

// mergeInstance moves the instance's props, node fields and non-template events onto the expanded root.
func mergeInstance(root, w *Node) {
	for k, v := range w.P {
		if root.P == nil {
			root.P = map[string]json.RawMessage{}
		}
		root.P[k] = v
	}
	for k, v := range w.Anim {
		if root.Anim == nil {
			root.Anim = map[string]json.RawMessage{}
		}
		root.Anim[k] = v
	}
	for ev, body := range w.On {
		if root.On == nil {
			root.On = map[string]json.RawMessage{}
		}
		root.On[ev] = concat(root.On[ev], body)
	}
	if w.When != nil {
		root.When = w.When
	}
	if w.Rep != nil {
		root.Rep = w.Rep
	}
	if w.Tr != nil {
		root.Tr = w.Tr
	}
	if w.Match != nil {
		root.Match = w.Match
	}
	if w.Drag != nil {
		root.Drag = w.Drag
	}
	if a := w.A11y; a != nil {
		if root.A11y == nil {
			root.A11y = &A11y{}
		}
		if a.Label != nil {
			root.A11y.Label = a.Label
		}
		if a.Role != "" {
			root.A11y.Role = a.Role
		}
		if a.Hidden != nil {
			root.A11y.Hidden = a.Hidden
		}
	}
}

func concat(a, b json.RawMessage) json.RawMessage {
	if a == nil {
		return b
	}
	return encode(append(asList(decode(a)), asList(decode(b))...))
}

// rewriter walks wire JSON. Qualify mode (template compile) prefixes the template's own instance state and fns.
// Expand mode (instance) substitutes params, inlines computed and handlers, splices emits and the slot, renames
// locals with a hygiene prefix and maps ids, drivers and timeline names. Substituted outer JSON is never walked.
type rewriter struct {
	qualify string

	expand   bool
	pfx      string
	params   map[string]json.RawMessage
	computed map[string]json.RawMessage
	handlers map[string]dsl.Body
	emits    map[string][]json.RawMessage
	slot     []*Node
	id       func(string) string
	tl       func(string) string
	fresh    func(tmpl string) string
	tmpl     string
}

// q qualifies the template's own names; nested templates' names already contain a dot.
func (r *rewriter) q(name string) string {
	if r.qualify == "" || strings.Contains(name, ".") {
		return name
	}
	return r.qualify + "." + name
}

func (r *rewriter) local(name string) string {
	if !r.expand || name == "event" {
		return name
	}
	return r.pfx + name
}

func (r *rewriter) mapID(id string) string {
	if !r.expand {
		return id
	}
	return r.id(id)
}

func (r *rewriter) exprRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return encode(r.expr(decode(raw)))
}

func (r *rewriter) stmtsRaw(raw json.RawMessage) json.RawMessage {
	return encode(r.stmts(asList(decode(raw))))
}

func (r *rewriter) exprs(v any) []any {
	out := []any{}
	for _, e := range asList(v) {
		out = append(out, r.expr(e))
	}
	return out
}

func (r *rewriter) expr(v any) any {
	a := asList(v)
	name, _ := a[1].(string)
	switch a[0] {
	case "v", "s", "d", "e", "t", "self":
		return a
	case "i":
		return []any{"i", r.q(name)}
	case "l":
		return []any{"l", r.local(name)}
	case "p":
		if w, ok := r.params[name]; ok && r.expand {
			return decode(w)
		}
		return a
	case "c":
		if body, ok := r.computed[name]; ok && r.expand {
			return r.expr(decode(body))
		}
		return a
	case "dr":
		return []any{"dr", a[1], r.mapID(a[2].(string)), a[3]}
	case "get":
		return []any{"get", r.expr(a[1]), a[2]}
	case "at":
		return []any{"at", r.expr(a[1]), r.expr(a[2])}
	case "op":
		return []any{"op", a[1], r.expr(a[2]), r.expr(a[3])}
	case "un":
		return []any{"un", a[1], r.expr(a[2])}
	case "if":
		return []any{"if", r.expr(a[1]), r.expr(a[2]), r.expr(a[3])}
	case "f":
		return []any{"f", a[1], r.exprs(a[2])}
	case "u":
		return []any{"u", r.q(name), r.exprs(a[2])}
	case "lam":
		var ps []any
		for _, p := range asList(a[1]) {
			ps = append(ps, r.local(p.(string)))
		}
		if ps == nil {
			ps = []any{}
		}
		return []any{"lam", ps, r.expr(a[2])}
	case "list", "str":
		return []any{a[0], r.exprs(a[1])}
	case "rec":
		out := map[string]any{}
		for k, e := range a[1].(map[string]any) {
			out[k] = r.expr(e)
		}
		return []any{"rec", out}
	}
	panic(fmt.Sprintf("compile: unknown wire expression %v", a[0]))
}

func (r *rewriter) stmts(ss []any) []any {
	out := []any{}
	for _, s := range ss {
		a := asList(s)
		switch a[0] {
		case "set":
			name := a[2].(string)
			if a[1] == "i" {
				name = r.q(name)
			}
			out = append(out, []any{"set", a[1], name, r.exprs(a[3]), r.expr(a[4])})
		case "let":
			out = append(out, []any{"let", r.local(a[1].(string)), r.expr(a[2])})
		case "if":
			out = append(out, []any{"if", r.expr(a[1]), r.stmts(asList(a[2])), r.stmts(asList(a[3]))})
		case "anim":
			out = append(out, []any{"anim", r.expr(a[1]), r.stmts(asList(a[2]))})
		case "call":
			res := a[3]
			if res != nil {
				res = r.local(res.(string))
			}
			out = append(out, []any{"call", a[1], r.expr(a[2]), res, a[4]})
		case "do":
			args := r.exprs(a[2])
			if (a[1] == "play" || a[1] == "stop") && r.expand {
				args = []any{[]any{"v", r.tl(asList(args[0])[1].(string))}}
			}
			out = append(out, []any{"do", a[1], args, a[3]})
		case "run":
			h, ok := r.handlers[a[1].(string)]
			if !ok || !r.expand {
				out = append(out, []any{"run", a[1], r.exprs(a[2])})
				continue
			}
			// inline the template handler (acyclic: dsl rejects handler recursion); params become fresh lets
			in := *r
			in.pfx = r.fresh(r.tmpl)
			for i, arg := range asList(a[2]) {
				out = append(out, []any{"let", in.local(h.Params[i]), r.expr(arg)})
			}
			out = append(out, in.stmts(asList(decode(h.Body)))...)
		case "emit":
			switch body := r.emits[a[1].(string)]; {
			case !r.expand && len(a) > 2:
				out = append(out, []any{"emit", a[1], r.expr(a[2])})
			case !r.expand:
				out = append(out, a)
			case len(a) > 2 && len(body) > 0:
				// bind the payload to a hygienic local so the instance's `event` cannot clash with the event of
				// the template handler that emits (e.g. a textField change inside the template)
				local := r.fresh(r.tmpl) + "event"
				out = append(out, []any{"let", local, r.expr(a[2])})
				for _, st := range body {
					out = append(out, renameEvent(decode(st), local))
				}
			default:
				for _, st := range body {
					out = append(out, decode(st))
				}
			}
		default:
			panic(fmt.Sprintf("compile: unknown wire statement %v", a[0]))
		}
	}
	return out
}

// node rewrites a template tree node; the slot expands to the instance's children.
func (r *rewriter) node(n *Node) []*Node {
	if n.T == "slot" && r.expand {
		return r.slot
	}
	m := &Node{T: n.T, ID: r.mapID(n.ID), Tr: n.Tr, Drag: n.Drag, When: r.exprRaw(n.When), Match: r.exprRaw(n.Match)}
	m.P = r.exprMap(n.P)
	m.Anim = r.exprMap(n.Anim)
	if n.Rep != nil {
		m.Rep = &Repeat{Over: r.exprRaw(n.Rep.Over), As: r.local(n.Rep.As), Index: r.local(n.Rep.Index), Key: r.exprRaw(n.Rep.Key)}
	}
	for _, ev := range sortedKeys(n.On) { // sorted: handler inlining draws hygiene prefixes in a stable order
		if m.On == nil {
			m.On = map[string]json.RawMessage{}
		}
		m.On[ev] = r.stmtsRaw(n.On[ev])
	}
	if n.Inst != nil {
		m.Inst = &Inst{State: map[string]json.RawMessage{}}
		for k, v := range n.Inst.State {
			m.Inst.State[r.q(k)] = r.exprRaw(v)
		}
	}
	if a := n.A11y; a != nil {
		m.A11y = &A11y{Label: r.exprRaw(a.Label), Role: a.Role, Hidden: r.exprRaw(a.Hidden)}
	}
	for _, c := range n.C {
		m.C = append(m.C, r.node(c)...)
	}
	return []*Node{m}
}

func (r *rewriter) exprMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	if m == nil {
		return nil
	}
	out := map[string]json.RawMessage{}
	for k, v := range m {
		out[k] = r.exprRaw(v)
	}
	return out
}

// renameEvent rewrites the local ["l", "event"] of outer statements to the emit's payload local. Literal
// payloads (["v", …]) are the only other place such an array could appear, so they are skipped.
func renameEvent(v any, to string) any {
	switch x := v.(type) {
	case []any:
		if len(x) == 2 && x[0] == "l" && x[1] == "event" {
			return []any{"l", to}
		}
		if len(x) > 0 && x[0] == "v" {
			return x
		}
		for i := range x {
			x[i] = renameEvent(x[i], to)
		}
	case map[string]any:
		for k := range x {
			x[k] = renameEvent(x[k], to)
		}
	}
	return v
}
