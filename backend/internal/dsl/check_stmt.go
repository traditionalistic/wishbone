package dsl

import "fmt"

// Statement checking (handler bodies and on.* values), split from check.go for size.

func (c *ctx) stmts(ss []Stmt) {
	c = c.push(map[string]*Type{})
	for _, s := range ss {
		c.stmt(s)
	}
}

// declare adds a handler local. Runtimes keep one locals chain per run (if/anim bodies share it), so a
// statement local may not shadow any visible local: params, repeat vars, outer lets.
func (c *ctx) declare(pos Pos, name string, t *Type) {
	if c.isLocal(name) {
		c.errf(pos, "duplicate local '%s' (a local of that name is already visible)", name)
	}
	c.sc.vars[name] = t
}

func (c *ctx) stmt(s Stmt) {
	switch s := s.(type) {
	case *LetStmt:
		c.declare(s.P, s.Name, c.declType(s.Value, nil, "'"+s.Name+"'", s.P))
	case *Assign:
		c.assign(s)
	case *IfStmt:
		if t := c.expr(s.Cond, nil); t != nil && !isKind(t, KBool) {
			c.typeErr(s.Cond.Pos(), "condition must be bool, got %s", t)
		}
		c.stmts(s.Then)
		c.stmts(s.Else)
	case *AnimStmt:
		c.want(s.Motion, TMotion, "animate")
		c.stmts(s.Body)
	case *CallStmt:
		op := c.p.Env.Ops[s.Op]
		if op == nil {
			c.errf(s.OpPos, "unknown operation '%s'", s.Op)
		} else {
			c.opArgs(op, s.Args, s.OpPos)
		}
		if s.Result != "" {
			var t *Type
			if op != nil {
				t = subst(c.p.Env.Stdlib.CallResult, map[string]*Type{"op result": op.Result})
			}
			c.declare(s.P, s.Result, t)
		}
	case *ExprStmt:
		c.exprStmt(s)
	case *EmitStmt:
		if !c.p.Env.Template {
			c.errf(s.P, "emit is only allowed in templates")
		} else if _, ok := c.p.decls[s.Name].(*EventDecl); !ok {
			c.errf(s.P, "emit of undeclared event '%s'", s.Name)
		}
	}
}

func (c *ctx) opArgs(op *Op, args Expr, pos Pos) {
	if args == nil {
		if empty := (&Type{Kind: KRecord}); !Assignable(empty, op.Args) {
			c.typeErr(pos, "arguments of %s: %s", op.ID, cannotUse(empty, op.Args))
		}
		return
	}
	c.want(args, op.Args, "arguments of "+op.ID)
}

func (c *ctx) assign(s *Assign) {
	var path []Expr
	x := s.Target
	for {
		if m, ok := x.(*Member); ok {
			path, x = append(path, m), m.X
		} else if ix, ok := x.(*Index); ok {
			path, x = append(path, ix), ix.X
		} else {
			break
		}
	}
	root, ok := x.(*Ident)
	if !ok {
		c.errf(s.P, "cannot assign to this expression; the target must be state with an optional .field / [index] path")
		c.expr(s.Value, nil)
		return
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	s.Root, s.Path = root.Name, path
	var t *Type
	local := c.isLocal(root.Name) || root.Name == "event" && c.event != nil
	switch d := c.p.decls[root.Name]; {
	case !local && isKindOf[*StateDecl](d):
		t, s.Inst = d.(*StateDecl).T, c.p.Env.Template
	case !local && d == nil:
		c.errf(root.P, "unknown name '%s'", root.Name)
		c.expr(s.Value, nil)
		return
	default:
		what := "local"
		if !local {
			what = declKind(d)
		}
		c.errf(root.P, "cannot assign to %s '%s'; only state is assignable", what, root.Name)
		c.expr(s.Value, nil)
		return
	}
	for _, seg := range path {
		if t == nil {
			break
		}
		t = nonOpt(t)
		switch seg := seg.(type) {
		case *Member:
			f := (*Field)(nil)
			if t.Kind == KRecord {
				f = t.field(seg.Name)
			}
			if f == nil {
				c.errf(seg.P, "unknown field '%s' on %s", seg.Name, t)
				t = nil
				break
			}
			seg.Kind, t = MemField, f.T
			if f.Opt {
				t = OptOf(t)
			}
		case *Index:
			switch t.Kind {
			case KList:
				c.want(seg.I, TNumber, "list index")
			case KMap:
				c.want(seg.I, TString, "map key")
			default:
				c.typeErr(seg.P, "cannot index %s (only lists and maps)", t)
				t = nil
				continue
			}
			t = t.Elem
		}
	}
	if t == nil {
		c.expr(s.Value, nil)
		return
	}
	c.want(s.Value, t, "assignment to "+root.Name)
}

func (c *ctx) exprStmt(s *ExprStmt) {
	e := s.Call
	if id, ok := e.Fn.(*Ident); ok && !c.isLocal(id.Name) {
		switch d := c.p.decls[id.Name].(type) {
		case *HandlerDecl:
			if s.Await {
				c.errf(s.P, "await is only allowed before call and play")
			}
			e.Kind, e.Name = CallHandler, d.Name
			if len(e.Args) != len(d.Params) {
				c.typeErr(e.P, "handler '%s' takes %d arguments, got %d", d.Name, len(d.Params), len(e.Args))
				return
			}
			for i, a := range e.Args {
				c.want(a, d.Params[i].T, fmt.Sprintf("argument %d of %s", i+1, d.Name))
			}
			return
		case nil:
			if f := c.p.Env.Stdlib.Effects[id.Name]; f != nil {
				c.effect(s, f)
				return
			}
		}
	}
	if c.expr(e, nil) != nil {
		c.errf(s.P, "the result of this call is unused; statements are assignments, handler calls, effects and `call`")
	}
}

func (c *ctx) effect(s *ExprStmt, f *Func) {
	e, sig := s.Call, f.Sigs[0]
	e.Kind, e.Name = CallEffect, f.Name
	if s.Await && f.Name != "play" {
		c.errf(s.P, "await is only allowed before call and play")
	}
	min := 0
	for _, p := range sig.Params {
		if !p.Opt {
			min++
		}
	}
	if len(e.Args) < min || len(e.Args) > len(sig.Params) {
		c.typeErr(e.P, "wrong number of arguments for %s: want %s, got %d", f.Name, arity(min, len(sig.Params)), len(e.Args))
		return
	}
	for i, a := range e.Args {
		pt := sig.Params[i].T
		if pt.Kind != KVar { // <data name> / <timeline name>: a bare name, not a value
			c.want(a, pt, fmt.Sprintf("argument %d of %s", i+1, f.Name))
			continue
		}
		id, _ := a.(*Ident)
		switch {
		case pt.Name == "data name" && (id == nil || !isKindOf[*DataDecl](c.p.decls[id.Name])):
			c.typeErr(a.Pos(), "%s needs the name of a data source", f.Name)
		case pt.Name == "timeline name" && id == nil:
			c.typeErr(a.Pos(), "%s needs the name of a timeline", f.Name)
		case pt.Name == "timeline name" && !c.p.Env.Timelines[id.Name]:
			c.errf(a.Pos(), "unknown timeline '%s'", id.Name)
		case id != nil:
			id.Ref = RefBare
		}
	}
}

func isKindOf[T Decl](d Decl) bool {
	_, ok := d.(T)
	return ok
}

func declKind(d Decl) string {
	switch d.(type) {
	case *StateDecl:
		return "state"
	case *LetDecl:
		return "computed"
	case *FnDecl:
		return "fn"
	case *HandlerDecl:
		return "handler"
	case *DataDecl:
		return "data"
	case *ParamDecl:
		return "param"
	}
	return "event"
}
