package dsl

import (
	"fmt"
	"strings"
)

// Env is what the caller (the screen compiler) knows beyond the script itself.
type Env struct {
	Stdlib    *Stdlib
	Ops       map[string]*Op
	Tokens    map[string]map[string]bool // group (color type space radius motion icon) → valid names
	Timelines map[string]bool            // names valid in play/stop
	Drivers   func(kind, id string) bool // kind "scroll" | "drag": is there such a node
	Template  bool                       // template script: state is instance state, `event`/`emit` allowed
}

// Scope is the per-node context of an expression or an on.* body.
type Scope struct {
	Locals    map[string]*Type // repeat vars etc., read as ["l", name]
	Event     *Type            // payload type inside on.*; nil = `event` is not available
	NoPayload string           // instance handler of this payload-less template event (for the error message)
	Self      bool             // self.pressed is available (node has on.tap)
}

// Program is a checked script: the environment for node expressions and the input of lowering.
type Program struct {
	Env    *Env
	Script *Script
	decls  map[string]Decl
	order  []Decl // registered decls in source order (duplicates dropped)
	stateN map[string]int
	mark   map[Decl]uint8 // lazy check of let/fn: 1 in progress, 2 done
	stack  []string
	diags  []Diag
}

var tokenGroups = map[string]*Type{"color": TColor, "type": TTextStyle, "space": TNumber, "radius": TNumber, "motion": TMotion, "icon": TIcon}

var reserved = map[string]bool{"env": true, "self": true, "scroll": true, "drag": true}

// Check type-checks the declarations of a script. Node expressions are checked afterwards with CheckExpr/CheckStmts.
func Check(s *Script, env *Env) (*Program, []Diag) {
	p := &Program{Env: env, Script: s, decls: map[string]Decl{}, stateN: map[string]int{}, mark: map[Decl]uint8{}}
	for _, d := range s.Decls {
		name := d.DeclName()
		switch {
		case reserved[name]:
			p.errf(d.Pos(), "'%s' is a reserved name", name)
			continue
		case p.decls[name] != nil:
			p.errf(d.Pos(), "duplicate declaration '%s'", name)
			continue
		}
		if ev, ok := d.(*EventDecl); ok {
			if !env.Template {
				p.errf(d.Pos(), "event declarations are only allowed in templates")
			}
			p.ctx(visNone, &p.diags).paramVars(ev.Fields) // duplicate payload fields
		}
		if _, ok := d.(*StateDecl); ok {
			p.stateN[name] = len(p.stateN)
		}
		p.decls[name] = d
		p.order = append(p.order, d)
	}
	for _, d := range p.order {
		switch d := d.(type) {
		case *ParamDecl:
			if d.Default != nil {
				c := p.ctx(visNone, &p.diags)
				c.want(d.Default, d.T, "default of param '"+d.Name+"'")
			}
		case *DataDecl:
			p.checkData(d)
		}
	}
	for _, d := range p.order {
		if d, ok := d.(*FnDecl); ok {
			p.ensureFn(d, d.P)
		}
	}
	for _, d := range p.order {
		if d, ok := d.(*StateDecl); ok {
			c := p.ctx(visState, &p.diags)
			c.stateN = p.stateN[d.Name]
			d.T = c.declType(d.Init, d.Ann, "state '"+d.Name+"'", d.P)
		}
	}
	for _, d := range p.order {
		if d, ok := d.(*LetDecl); ok {
			p.ensureLet(d, d.P)
		}
	}
	for _, d := range p.order {
		if d, ok := d.(*HandlerDecl); ok {
			c := p.ctx(visAll, &p.diags)
			c.push(c.paramVars(d.Params)).stmts(d.Body)
		}
	}
	for _, d := range p.order {
		if d, ok := d.(*HandlerDecl); ok {
			p.ensureHandler(d, d.P)
		}
	}
	return p, p.diags
}

// ensureHandler rejects handler-call cycles: execution must be finite (spec §3.7).
func (p *Program) ensureHandler(d *HandlerDecl, pos Pos) {
	if !p.enter(d, d.Name, pos, "handler") {
		return
	}
	defer p.leave(d)
	var walk func(ss []Stmt)
	walk = func(ss []Stmt) {
		for _, s := range ss {
			switch s := s.(type) {
			case *ExprStmt:
				if h, ok := p.decls[s.Call.Name].(*HandlerDecl); ok && s.Call.Kind == CallHandler {
					p.ensureHandler(h, s.P)
				}
			case *IfStmt:
				walk(s.Then)
				walk(s.Else)
			case *AnimStmt:
				walk(s.Body)
			}
		}
	}
	walk(d.Body)
}

// CheckExpr checks a node expression; want (optional) is the expected type, e.g. the prop type.
func (p *Program) CheckExpr(e Expr, sc *Scope, want *Type) []Diag {
	var diags []Diag
	c := p.scopeCtx(sc, &diags)
	if want != nil {
		c.want(e, want, "")
	} else {
		c.expr(e, nil)
	}
	return diags
}

// CheckStmts checks an on.* body.
func (p *Program) CheckStmts(ss []Stmt, sc *Scope) []Diag {
	var diags []Diag
	p.scopeCtx(sc, &diags).stmts(ss)
	return diags
}

func (p *Program) errf(pos Pos, format string, a ...any) {
	p.diags = append(p.diags, Diag{pos.Line, pos.Col, fmt.Sprintf(format, a...)})
}

func (p *Program) scopeCtx(sc *Scope, diags *[]Diag) *ctx {
	c := p.ctx(visAll, diags)
	if sc != nil {
		c = c.push(sc.Locals)
		c.event, c.self, c.noPayload = sc.Event, sc.Self, sc.NoPayload
	}
	return c
}

func (p *Program) checkData(d *DataDecl) {
	op := p.Env.Ops[d.Op]
	if op == nil {
		p.errf(d.OpPos, "unknown operation '%s'", d.Op)
		return
	}
	if op.Kind != "read" {
		p.errf(d.OpPos, "operation '%s' is %s; data sources need a read operation", d.Op, op.Kind)
	}
	c := p.ctx(visData, &p.diags)
	c.opArgs(op, d.Args, d.OpPos)
	if d.Args != nil {
		c.literalOnly(d.Args)
	}
	d.T = op.Result
	if !d.Required {
		d.T = OptOf(op.Result)
	}
}

func (p *Program) enter(d Decl, name string, pos Pos, what string) bool {
	switch p.mark[d] {
	case 2:
		return false
	case 1:
		i := 0
		for p.stack[i] != name {
			i++
		}
		path := strings.Join(append(append([]string{}, p.stack[i:]...), name), " → ")
		if what != "let" {
			p.errf(pos, "%s '%s' is recursive (%s); recursion is not allowed", what, name, path)
		} else {
			p.errf(pos, "cycle in computed values: %s", path)
		}
		return false
	}
	p.mark[d] = 1
	p.stack = append(p.stack, name)
	return true
}

func (p *Program) leave(d Decl) {
	p.mark[d] = 2
	p.stack = p.stack[:len(p.stack)-1]
}

func (p *Program) ensureFn(d *FnDecl, pos Pos) {
	if !p.enter(d, d.Name, pos, "fn") {
		return
	}
	defer p.leave(d)
	c := p.ctx(visFn, &p.diags)
	d.Ret = c.push(c.paramVars(d.Params)).declType(d.Body, d.Ret, "fn '"+d.Name+"'", d.P)
}

func (p *Program) ensureLet(d *LetDecl, pos Pos) {
	if !p.enter(d, d.Name, pos, "let") {
		return
	}
	defer p.leave(d)
	d.T = p.ctx(visAll, &p.diags).declType(d.Value, d.Ann, "'"+d.Name+"'", d.P)
}

// vis restricts which top-level names an expression may read.
type vis uint8

const (
	visAll   vis = iota // computed, handlers, node expressions
	visState            // state initializers: params, data, earlier state, fns
	visFn               // fn bodies: fns only
	visData             // data args: params only
	visNone             // param defaults
)

var visWhy = [...]string{
	visState: "state initializers see params, data, earlier state and fns",
	visFn:    "fn bodies see only their parameters and other fns",
	visData:  "data arguments may only reference params",
	visNone:  "param defaults cannot reference declarations",
}

type ctx struct {
	p         *Program
	diags     *[]Diag
	sc        *scope
	vis       vis
	stateN    int // visState: states declared before this index are visible
	event     *Type
	noPayload string
	self      bool
}

type scope struct {
	up   *scope
	vars map[string]*Type
}

func (s *scope) lookup(name string) (*Type, bool) {
	for ; s != nil; s = s.up {
		if t, ok := s.vars[name]; ok {
			return t, true
		}
	}
	return nil, false
}

func (p *Program) ctx(v vis, diags *[]Diag) *ctx { return &ctx{p: p, diags: diags, vis: v} }

func (c *ctx) push(vars map[string]*Type) *ctx {
	c2 := *c
	c2.sc = &scope{up: c.sc, vars: vars}
	return &c2
}

func (c *ctx) errf(pos Pos, format string, a ...any) {
	*c.diags = append(*c.diags, Diag{pos.Line, pos.Col, fmt.Sprintf(format, a...)})
}

func (c *ctx) typeErr(pos Pos, format string, a ...any) { c.errf(pos, "type error: "+format, a...) }

func (c *ctx) isLocal(name string) bool {
	_, ok := c.sc.lookup(name)
	return ok
}

func (c *ctx) paramVars(ps []ParamDef) map[string]*Type {
	vars := map[string]*Type{}
	for _, p := range ps {
		if _, dup := vars[p.Name]; dup {
			c.errf(p.P, "duplicate parameter '%s'", p.Name)
		}
		vars[p.Name] = p.T
	}
	return vars
}

// want checks e against an expected type.
func (c *ctx) want(e Expr, want *Type, what string) *Type {
	t := c.expr(e, want)
	if t != nil && !Assignable(t, want) {
		if what != "" {
			what += ": "
		}
		c.typeErr(e.Pos(), "%s%s", what, cannotUse(t, want))
	}
	return t
}

// declType is the type of a declaration: its annotation (checked) or the inferred type.
func (c *ctx) declType(e Expr, ann *Type, what string, pos Pos) *Type {
	if ann != nil {
		c.want(e, ann, what)
		return ann
	}
	t := c.expr(e, nil)
	if isKind(t, KNull) {
		c.errf(pos, "cannot infer the type of %s from null; add a type annotation", what)
		return nil
	}
	return t
}

// --- expressions ---

func (c *ctx) expr(e Expr, want *Type) *Type {
	t := c.expr0(e, want)
	e.base().T = t
	return t
}

func (c *ctx) expr0(e Expr, want *Type) *Type {
	switch e := e.(type) {
	case *Lit:
		switch v := e.Val.(type) {
		case float64:
			return TNumber
		case bool:
			return TBool
		case string:
			if hasLit(want) {
				return LitOf(v)
			}
			return TString
		}
		return TNull
	case *Interp:
		for _, part := range e.Parts {
			c.expr(part, nil)
		}
		return TString
	case *Ident:
		return c.ident(e)
	case *TokenRef:
		return c.token(e)
	case *Member:
		return c.member(e)
	case *Index:
		return c.index(e)
	case *Call:
		return c.call(e)
	case *Unary:
		return c.unary(e)
	case *Binary:
		return c.binary(e, want)
	case *Cond:
		return c.cond(e, want)
	case *ListLit:
		return c.list(e, want)
	case *RecLit:
		return c.record(e, want)
	case *Lambda:
		c.errf(e.P, "a lambda is only allowed as an argument of a stdlib function")
		return nil
	}
	panic(fmt.Sprintf("dsl: unknown expression %T", e))
}

func (c *ctx) ident(e *Ident) *Type {
	if t, ok := c.sc.lookup(e.Name); ok {
		e.Ref = RefLocal
		return t
	}
	if e.Name == "event" {
		if c.event != nil {
			e.Ref = RefLocal
			return c.event
		}
		if c.noPayload != "" {
			c.errf(e.P, "template event '%s' has no payload; declare it as `event %s(name: Type)` and emit it with a payload", c.noPayload, c.noPayload)
		} else {
			c.errf(e.P, "'event' is only available inside on.* handlers")
		}
		return nil
	}
	hidden := func() *Type {
		c.errf(e.P, "unknown name '%s' (%s)", e.Name, visWhy[c.vis])
		return nil
	}
	switch d := c.p.decls[e.Name].(type) {
	case nil:
		if reserved[e.Name] {
			c.errf(e.P, "'%s' is not a value; use %s.<name>", e.Name, e.Name)
		} else {
			c.errf(e.P, "unknown name '%s'", e.Name)
		}
		return nil
	case *ParamDecl:
		if c.vis == visFn || c.vis == visNone {
			return hidden()
		}
		e.Ref = RefParam
		return d.T
	case *DataDecl:
		if c.vis != visAll && c.vis != visState {
			return hidden()
		}
		e.Ref = RefData
		return d.T
	case *StateDecl:
		if c.vis == visState && c.p.Env.Template {
			// instance state travels as a JSON object (inst.state): clients initialize it in no particular order
			c.errf(e.P, "a template state initializer cannot read state '%s': instance state has no initialization order", e.Name)
			return d.T
		}
		if c.vis != visAll && (c.vis != visState || c.p.stateN[e.Name] >= c.stateN) {
			return hidden()
		}
		e.Ref = RefState
		if c.p.Env.Template {
			e.Ref = RefInst
		}
		return d.T
	case *LetDecl:
		if c.vis != visAll {
			return hidden()
		}
		c.p.ensureLet(d, e.P)
		e.Ref = RefComputed
		return d.T
	case *FnDecl:
		c.typeErr(e.P, "fn '%s' is not a value; call it", e.Name)
	case *HandlerDecl:
		c.typeErr(e.P, "handler '%s' is not a value", e.Name)
	case *EventDecl:
		c.typeErr(e.P, "event '%s' is not a value; use emit %s", e.Name, e.Name)
	}
	return nil
}

func (c *ctx) token(e *TokenRef) *Type {
	t, ok := tokenGroups[e.Group]
	switch {
	case !ok:
		c.errf(e.P, "unknown token group '@%s' (color, type, space, radius, motion, icon)", e.Group)
		return nil
	case !c.p.Env.Tokens[e.Group][e.Name] && e.Group == "icon":
		c.errf(e.P, "unknown icon '%s'", e.Name)
		return nil
	case !c.p.Env.Tokens[e.Group][e.Name]:
		c.errf(e.P, "unknown %s token '@%s.%s'", e.Group, e.Group, e.Name)
		return nil
	}
	return t
}

func (c *ctx) special(x Expr, names ...string) (string, bool) {
	id, ok := x.(*Ident)
	if !ok || c.isLocal(id.Name) {
		return "", false
	}
	for _, n := range names {
		if id.Name == n {
			return n, true
		}
	}
	return "", false
}

func (c *ctx) member(e *Member) *Type {
	switch n, _ := c.special(e.X, "env", "self", "scroll", "drag"); n {
	case "env":
		t := c.p.Env.Stdlib.Env[e.Name]
		if t == nil {
			c.errf(e.P, "unknown env field 'env.%s'", e.Name)
			return nil
		}
		e.Kind = MemEnv
		return t
	case "self":
		if e.Name != "pressed" || !c.self {
			c.errf(e.P, "unknown name 'self.%s' (self.pressed exists only on nodes with on.tap)", e.Name)
			return nil
		}
		e.Kind = MemSelf
		return TBool
	case "scroll", "drag":
		c.errf(e.P, "%s drivers are read as %s.<id>.x or %s.<id>.y", n, n, n)
		return nil
	}
	if inner, ok := e.X.(*Member); ok {
		if kind, ok := c.special(inner.X, "scroll", "drag"); ok {
			switch {
			case c.vis != visAll:
				c.errf(inner.X.Pos(), "unknown name '%s' (%s)", kind, visWhy[c.vis])
				return nil
			case e.Name != "x" && e.Name != "y":
				c.errf(e.P, "unknown %s driver axis '%s' (x or y)", kind, e.Name)
				return nil
			case c.p.Env.Drivers == nil || !c.p.Env.Drivers(kind, inner.Name):
				c.errf(inner.P, "unknown %s driver '%s' (no such node)", kind, inner.Name)
				return nil
			}
			e.Kind = MemDriver
			return TNumber
		}
	}
	xt := c.expr(e.X, nil)
	if xt == nil {
		return nil
	}
	recv := nonOpt(xt)
	if recv.Kind == KRecord {
		if f := recv.field(e.Name); f != nil {
			e.Kind = MemField
			if f.Opt || xt.Kind == KOpt {
				return OptOf(f.T)
			}
			return f.T
		}
	}
	if f := c.p.Env.Stdlib.Funcs[e.Name]; f != nil && f.Property && recv.Kind != KNull {
		e.Kind = MemProp
		t := c.stdCall(e.P, f, []Expr{e.X}, recv)
		if xt.Kind == KOpt {
			t = OptOf(t)
		}
		return t
	}
	if f := c.p.Env.Stdlib.Funcs[e.Name]; f != nil && f.Method {
		c.typeErr(e.P, "method '%s' must be called: .%s(…)", e.Name, e.Name)
		return nil
	}
	c.errf(e.P, "unknown field '%s' on %s", e.Name, xt)
	return nil
}

func (c *ctx) index(e *Index) *Type {
	xt := c.expr(e.X, nil)
	if xt == nil {
		c.expr(e.I, nil)
		return nil
	}
	switch b := nonOpt(xt); b.Kind {
	case KList:
		c.want(e.I, TNumber, "list index")
		return OptOf(b.Elem)
	case KMap:
		c.want(e.I, TString, "map key")
		return OptOf(b.Elem)
	}
	c.expr(e.I, nil)
	c.typeErr(e.P, "cannot index %s (only lists and maps)", xt)
	return nil
}

func (c *ctx) call(e *Call) *Type {
	e.Recv = nil
	std := c.p.Env.Stdlib
	switch fn := e.Fn.(type) {
	case *Ident:
		local := c.isLocal(fn.Name)
		var decl Decl
		if !local {
			decl = c.p.decls[fn.Name]
		}
		switch d := decl.(type) {
		case *FnDecl:
			if c.vis == visData || c.vis == visNone {
				c.errf(fn.P, "unknown function '%s' (%s)", fn.Name, visWhy[c.vis])
				return nil
			}
			return c.fnCall(e, d)
		case *HandlerDecl:
			c.typeErr(e.P, "handler '%s' cannot be called in an expression; call it as a statement", fn.Name)
			return nil
		}
		// a value named like a stdlib function (`state count`) leaves the function callable
		if f := std.Funcs[fn.Name]; f != nil {
			e.Kind, e.Name = CallStd, f.Name
			return c.stdCall(fn.P, f, e.Args, nil)
		}
		switch {
		case local || decl != nil:
			c.typeErr(fn.P, "'%s' is not a function", fn.Name)
		case std.Effects[fn.Name] != nil:
			c.typeErr(e.P, "effect '%s' cannot be used in an expression; call it as a statement", fn.Name)
		default:
			c.errf(fn.P, "unknown function '%s'", fn.Name)
		}
		return nil
	case *Member:
		f := std.Funcs[fn.Name]
		if f == nil || !f.Method {
			if c.expr(fn.X, nil) != nil {
				c.errf(fn.P, "unknown method '%s'", fn.Name)
			}
			return nil
		}
		xt := c.expr(fn.X, nil)
		if xt == nil {
			return nil
		}
		recv := nonOpt(xt)
		if recv.Kind == KNull {
			c.typeErr(fn.P, "cannot call %s on null", fn.Name)
			return nil
		}
		e.Kind, e.Name, e.Recv = CallStd, f.Name, fn.X
		t := c.stdCall(fn.P, f, append([]Expr{fn.X}, e.Args...), recv)
		if xt.Kind == KOpt { // null-propagating method call
			t = OptOf(t)
		}
		return t
	}
	c.typeErr(e.P, "this expression is not callable")
	return nil
}

func (c *ctx) fnCall(e *Call, d *FnDecl) *Type {
	e.Kind, e.Name = CallFn, d.Name
	c.p.ensureFn(d, e.P)
	if len(e.Args) != len(d.Params) {
		c.typeErr(e.P, "fn '%s' takes %d arguments, got %d", d.Name, len(d.Params), len(e.Args))
		return d.Ret
	}
	for i, a := range e.Args {
		c.want(a, d.Params[i].T, fmt.Sprintf("argument %d of %s", i+1, d.Name))
	}
	return d.Ret
}

func (c *ctx) unary(e *Unary) *Type {
	xt := c.expr(e.X, nil)
	if e.Op == "!" {
		if xt != nil && !isKind(xt, KBool) {
			c.typeErr(e.P, "'!' needs bool, got %s", xt)
		}
		return TBool
	}
	if xt != nil && !isKind(xt, KNumber) {
		c.typeErr(e.P, "unary '-' needs number, got %s", xt)
	}
	return TNumber
}

func (c *ctx) binary(e *Binary, want *Type) *Type {
	if e.Op == "??" {
		lt := c.expr(e.X, OptOf(want))
		rw := want
		if rw == nil && lt != nil && lt.Kind != KNull {
			rw = nonOpt(lt)
		}
		rt := c.expr(e.Y, rw)
		switch {
		case lt == nil || rt == nil:
			return nil
		case lt.Kind == KNull:
			return rt
		}
		if j := join(nonOpt(lt), rt); j != nil {
			return j
		}
		c.typeErr(e.P, "'??' operands have incompatible types %s and %s", lt, rt)
		return nil
	}
	lt, rt := c.expr(e.X, nil), c.expr(e.Y, nil)
	if lt == nil || rt == nil {
		return nil
	}
	num := isKind(lt, KNumber) && isKind(rt, KNumber)
	switch e.Op {
	case "+":
		switch {
		case num:
			return TNumber
		case stringy(lt) && stringy(rt):
			return TString
		}
		c.typeErr(e.P, "'+' needs number + number or string + string, got %s + %s", lt, rt)
		return nil
	case "-", "*", "/", "%":
		if !num {
			c.typeErr(e.P, "'%s' needs numbers, got %s and %s", e.Op, lt, rt)
		}
		return TNumber
	case "<", "<=", ">", ">=":
		if !num && !(stringy(lt) && stringy(rt)) {
			c.typeErr(e.P, "'%s' compares two numbers or two strings, got %s and %s", e.Op, lt, rt)
		}
	case "==", "!=":
		if join(lt, rt) == nil {
			c.typeErr(e.P, "cannot compare %s with %s", lt, rt)
		}
	case "&&", "||":
		if !isKind(lt, KBool) || !isKind(rt, KBool) {
			c.typeErr(e.P, "'%s' needs bool operands, got %s and %s", e.Op, lt, rt)
		}
	}
	return TBool
}

func (c *ctx) cond(e *Cond, want *Type) *Type {
	if ct := c.expr(e.C, nil); ct != nil && !isKind(ct, KBool) {
		c.typeErr(e.C.Pos(), "condition must be bool, got %s", ct)
	}
	at, bt := c.expr(e.A, want), c.expr(e.B, want)
	if at == nil || bt == nil {
		return nil
	}
	if j := join(at, bt); j != nil {
		return j
	}
	if want != nil && Assignable(at, want) && Assignable(bt, want) {
		return want
	}
	c.typeErr(e.P, "branches of '?:' have incompatible types %s and %s", at, bt)
	return nil
}

func (c *ctx) list(e *ListLit, want *Type) *Type {
	var ew *Type
	if l := pick(want, KList); l != nil {
		ew = l.Elem
	} else if isKind(nonOpt(want), KAny) { // a sink like map<any>: [] is list<any>
		ew = TAny
	}
	if len(e.Elems) == 0 {
		if ew == nil {
			c.errf(e.P, "an empty list needs a type annotation (e.g. state xs: list<number> = [])")
			return nil
		}
		return ListOf(ew)
	}
	ts := make([]*Type, len(e.Elems))
	for i, x := range e.Elems {
		if ts[i] = c.expr(x, ew); ts[i] == nil {
			return nil
		}
	}
	et := ts[0]
	for i, t := range ts[1:] {
		if j := join(et, t); j != nil {
			et = j
			continue
		}
		if ew != nil && all(ts, func(t *Type) bool { return Assignable(t, ew) }) {
			return ListOf(ew)
		}
		c.typeErr(e.Elems[i+1].Pos(), "list elements have incompatible types %s and %s", et, t)
		return nil
	}
	return ListOf(et)
}

func all(ts []*Type, f func(*Type) bool) bool {
	for _, t := range ts {
		if !f(t) {
			return false
		}
	}
	return true
}

func (c *ctx) record(e *RecLit, want *Type) *Type {
	w := pick(want, KRecord, KMap)
	r := &Type{Kind: KRecord}
	ok := true
	for _, f := range e.Fields {
		if r.field(f.Name) != nil {
			c.errf(f.P, "duplicate field '%s'", f.Name)
			continue
		}
		var fw *Type
		switch {
		case w == nil:
		case w.Kind == KMap:
			fw = w.Elem
		case w.field(f.Name) != nil:
			fw = w.field(f.Name).T
		}
		t := c.expr(f.X, fw)
		ok = ok && t != nil
		r.Fields = append(r.Fields, Field{Name: f.Name, T: t})
	}
	if !ok {
		return nil
	}
	return r
}

// literalOnly enforces that data arguments are plain JSON with params.
func (c *ctx) literalOnly(e Expr) {
	switch e := e.(type) {
	case *Lit, *Ident:
		return
	case *Unary:
		if _, ok := e.X.(*Lit); ok && e.Op == "-" {
			return
		}
	case *ListLit:
		for _, x := range e.Elems {
			c.literalOnly(x)
		}
		return
	case *RecLit:
		for _, f := range e.Fields {
			c.literalOnly(f.X)
		}
		return
	}
	c.errf(e.Pos(), "data arguments may only contain literals and params")
}
