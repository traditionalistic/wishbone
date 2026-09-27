package dsl

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Hooks let the screen compiler rewrite parts of the output (template expansion). Nil hooks = default wire forms.
type Hooks struct {
	// Ref may replace the read of a resolved name (e.g. a template param by the instance's argument); nil = default form.
	Ref func(kind RefKind, name string) json.RawMessage
	// Emit returns the statements inlined in place of `emit name`; without it `["emit", name]` is produced.
	Emit func(event string) []json.RawMessage
}

// Decls is the lowered script, in the shapes of the wire document (spec §3.8).
type Decls struct {
	Params   map[string]json.RawMessage `json:"params"` // default expression; null when the param is required
	State    []json.RawMessage          `json:"state"`  // ["name", expr] in declaration order
	Computed map[string]json.RawMessage `json:"computed"`
	Fns      map[string]Body            `json:"fns"`
	Handlers map[string]Body            `json:"handlers"`
	Data     []DataSource               `json:"data"`
	Events   []string                   `json:"events,omitempty"`
}

type Body struct {
	Params []string        `json:"params"`
	Body   json.RawMessage `json:"body"`
}

type DataSource struct {
	Name     string          `json:"name"`
	Op       string          `json:"op"`
	Args     json.RawMessage `json:"args"` // plain JSON; params as {"$param": name}
	Required bool            `json:"required"`
}

// LowerExpr lowers an expression checked without diagnostics.
func LowerExpr(e Expr, h *Hooks) json.RawMessage { return lowerer{h}.expr(e) }

// LowerStmts lowers a checked statement list to a JSON array.
func LowerStmts(ss []Stmt, h *Hooks) json.RawMessage { return enc(lowerer{h}.stmts(ss)) }

// Lower lowers the checked declarations.
func (p *Program) Lower(h *Hooks) *Decls {
	l := lowerer{h}
	d := &Decls{Params: map[string]json.RawMessage{}, State: []json.RawMessage{}, Computed: map[string]json.RawMessage{},
		Fns: map[string]Body{}, Handlers: map[string]Body{}, Data: []DataSource{}}
	for _, decl := range p.order {
		switch x := decl.(type) {
		case *ParamDecl:
			d.Params[x.Name] = json.RawMessage("null")
			if x.Default != nil {
				d.Params[x.Name] = l.expr(x.Default)
			}
		case *StateDecl:
			d.State = append(d.State, enc([]any{x.Name, l.expr(x.Init)}))
		case *LetDecl:
			d.Computed[x.Name] = l.expr(x.Value)
		case *FnDecl:
			d.Fns[x.Name] = Body{paramNames(x.Params), l.expr(x.Body)}
		case *HandlerDecl:
			d.Handlers[x.Name] = Body{paramNames(x.Params), enc(l.stmts(x.Body))}
		case *DataDecl:
			d.Data = append(d.Data, DataSource{x.Name, x.Op, dataJSON(x.Args), x.Required})
		case *EventDecl:
			d.Events = append(d.Events, x.Name)
		}
	}
	return d
}

func paramNames(ps []ParamDef) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}

// enc marshals without HTML escaping.
func enc(v any) json.RawMessage {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimRight(b.Bytes(), "\n")
}

// obj is a JSON object that keeps source key order.
type obj []kv

type kv struct {
	k string
	v json.RawMessage
}

func (o obj) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, x := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(enc(x.k))
		b.WriteByte(':')
		b.Write(x.v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

var refTags = [...]string{RefState: "s", RefInst: "i", RefComputed: "c", RefData: "d", RefLocal: "l", RefParam: "p"}

type lowerer struct{ h *Hooks }

func (l lowerer) exprs(es []Expr) []json.RawMessage {
	out := make([]json.RawMessage, len(es))
	for i, e := range es {
		out[i] = l.expr(e)
	}
	return out
}

func (l lowerer) ref(k RefKind, name string) json.RawMessage {
	switch {
	case k == RefBare:
		return enc([]any{"v", name})
	case k == RefNone:
		panic("dsl: lowering an unchecked name " + name)
	case l.h != nil && l.h.Ref != nil:
		if r := l.h.Ref(k, name); r != nil {
			return r
		}
	}
	return enc([]any{refTags[k], name})
}

func (l lowerer) expr(e Expr) json.RawMessage {
	switch e := e.(type) {
	case *Lit:
		return enc([]any{"v", e.Val})
	case *Interp:
		return enc([]any{"str", l.exprs(e.Parts)})
	case *Ident:
		return l.ref(e.Ref, e.Name)
	case *TokenRef:
		return enc([]any{"t", e.Group, e.Name})
	case *Member:
		switch e.Kind {
		case MemEnv:
			return enc([]any{"e", e.Name})
		case MemSelf:
			return enc([]any{"self", "pressed"})
		case MemDriver:
			in := e.X.(*Member)
			return enc([]any{"dr", in.X.(*Ident).Name, in.Name, e.Name})
		case MemProp:
			return enc([]any{"f", e.Name, []json.RawMessage{l.expr(e.X)}})
		}
		return enc([]any{"get", l.expr(e.X), e.Name})
	case *Index:
		return enc([]any{"at", l.expr(e.X), l.expr(e.I)})
	case *Call:
		args := e.Args
		if e.Recv != nil {
			args = append([]Expr{e.Recv}, e.Args...)
		}
		tag := "f"
		if e.Kind == CallFn {
			tag = "u"
		}
		return enc([]any{tag, e.Name, l.exprs(args)})
	case *Unary:
		return enc([]any{"un", e.Op, l.expr(e.X)})
	case *Binary:
		return enc([]any{"op", e.Op, l.expr(e.X), l.expr(e.Y)})
	case *Cond:
		return enc([]any{"if", l.expr(e.C), l.expr(e.A), l.expr(e.B)})
	case *ListLit:
		return enc([]any{"list", l.exprs(e.Elems)})
	case *RecLit:
		o := obj{}
		for _, f := range e.Fields {
			o = append(o, kv{f.Name, l.expr(f.X)})
		}
		return enc([]any{"rec", o})
	case *Lambda:
		return enc([]any{"lam", append([]string{}, e.Params...), l.expr(e.Body)})
	}
	panic(fmt.Sprintf("dsl: cannot lower %T", e))
}

func (l lowerer) stmts(ss []Stmt) []json.RawMessage {
	out := []json.RawMessage{}
	for _, s := range ss {
		if em, ok := s.(*EmitStmt); ok && l.h != nil && l.h.Emit != nil {
			out = append(out, l.h.Emit(em.Name)...)
			continue
		}
		out = append(out, l.stmt(s))
	}
	return out
}

func (l lowerer) stmt(s Stmt) json.RawMessage {
	switch s := s.(type) {
	case *Assign:
		scope := "s"
		if s.Inst {
			scope = "i"
		}
		path := []json.RawMessage{}
		for _, seg := range s.Path {
			if m, ok := seg.(*Member); ok {
				path = append(path, enc([]any{"v", m.Name}))
			} else {
				path = append(path, l.expr(seg.(*Index).I))
			}
		}
		return enc([]any{"set", scope, s.Root, path, l.expr(s.Value)})
	case *LetStmt:
		return enc([]any{"let", s.Name, l.expr(s.Value)})
	case *IfStmt:
		return enc([]any{"if", l.expr(s.Cond), l.stmts(s.Then), l.stmts(s.Else)})
	case *AnimStmt:
		return enc([]any{"anim", l.expr(s.Motion), l.stmts(s.Body)})
	case *CallStmt:
		args := enc([]any{"v", obj{}})
		if s.Args != nil {
			args = l.expr(s.Args)
		}
		var result any
		if s.Result != "" {
			result = s.Result
		}
		return enc([]any{"call", s.Op, args, result, s.Await})
	case *ExprStmt:
		args := l.exprs(s.Call.Args)
		if s.Call.Kind == CallHandler {
			return enc([]any{"run", s.Call.Name, args})
		}
		return enc([]any{"do", s.Call.Name, args, s.Await})
	case *EmitStmt:
		return enc([]any{"emit", s.Name})
	}
	panic(fmt.Sprintf("dsl: cannot lower %T", s))
}

// dataJSON lowers data arguments (literals and params only) to plain JSON.
func dataJSON(e Expr) json.RawMessage {
	switch e := e.(type) {
	case nil:
		return json.RawMessage("{}")
	case *Lit:
		return enc(e.Val)
	case *Unary:
		return enc(-e.X.(*Lit).Val.(float64))
	case *Ident:
		return enc(map[string]string{"$param": e.Name})
	case *ListLit:
		out := make([]json.RawMessage, len(e.Elems))
		for i, x := range e.Elems {
			out[i] = dataJSON(x)
		}
		return enc(out)
	case *RecLit:
		o := obj{}
		for _, f := range e.Fields {
			o = append(o, kv{f.Name, dataJSON(f.X)})
		}
		return enc(o)
	}
	panic(fmt.Sprintf("dsl: data argument %T", e))
}
