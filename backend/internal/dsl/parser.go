package dsl

import (
	"fmt"
	"strings"
)

var keywords = map[string]bool{"state": true, "let": true, "fn": true, "handler": true, "data": true, "param": true,
	"event": true, "if": true, "else": true, "true": true, "false": true, "null": true, "await": true, "animate": true, "emit": true}

var declKeywords = map[string]bool{"state": true, "let": true, "fn": true, "handler": true, "data": true, "param": true, "event": true}

// Binary operator precedences; ?: is 1 (spec §3.5).
var binPrec = map[string]int{"??": 2, "||": 3, "&&": 4, "==": 5, "!=": 5, "<": 6, "<=": 6, ">": 6, ">=": 6,
	"+": 7, "-": 7, "*": 8, "/": 8, "%": 8}

type parser struct {
	toks   []token
	i      int
	diags  *[]Diag
	sig    bool            // stdlib signature mode: <placeholders>, function types, generic vars
	vars   map[string]bool // generic vars in sig mode
	script bool            // user annotations: `any` is not allowed
	depth  int
}

// maxDepth bounds syntactic nesting: a Go stack overflow on hostile input is fatal, not recoverable.
const maxDepth = 500

// nest enters one nesting level; the caller must defer p.depth--.
func (p *parser) nest(pos Pos) {
	if p.depth++; p.depth > maxDepth {
		p.fail(pos, "expression nested too deeply (limit %d)", maxDepth)
	}
}

type bailout struct{}

func newParser(src string) *parser {
	toks, diags := lex(src)
	return &parser{toks: toks, diags: &diags}
}

func (p *parser) peek() token { return p.toks[p.i] }

func (p *parser) peekN(k int) token {
	if p.i+k < len(p.toks) {
		return p.toks[p.i+k]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != tEOF {
		p.i++
	}
	return t
}

func (p *parser) atEOF() bool { return p.peek().kind == tEOF }
func (p *parser) is(s string) bool {
	t := p.peek()
	return t.kind == tPunct && t.text == s
}
func (p *parser) isKw(s string) bool {
	t := p.peek()
	return t.kind == tIdent && t.text == s
}

func (p *parser) errAt(at Pos, format string, a ...any) {
	*p.diags = append(*p.diags, Diag{at.Line, at.Col, "syntax error: " + fmt.Sprintf(format, a...)})
}

func (p *parser) fail(at Pos, format string, a ...any) {
	p.errAt(at, format, a...)
	panic(bailout{})
}

// run executes f, turning a syntax-error bailout into ok=false.
func (p *parser) run(f func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, b := r.(bailout); !b {
				panic(r)
			}
			ok = false
		}
	}()
	f()
	return true
}

func desc(t token) string {
	switch t.kind {
	case tEOF:
		return "end of input"
	case tIdent:
		return "'" + t.text + "'"
	case tNum:
		return "number " + t.text
	case tStr:
		return "string"
	case tTok:
		return "token @" + t.text
	}
	return "'" + t.text + "'"
}

func (p *parser) expect(s string) token {
	if !p.is(s) {
		p.fail(p.peek().pos, "expected '%s', found %s", s, desc(p.peek()))
	}
	return p.next()
}

func (p *parser) name() (string, Pos) {
	t := p.peek()
	if t.kind != tIdent || keywords[t.text] {
		p.fail(t.pos, "expected a name, found %s", desc(t))
	}
	p.next()
	return t.text, t.pos
}

func (p *parser) endOfInput() {
	if !p.atEOF() {
		p.fail(p.peek().pos, "unexpected %s", desc(p.peek()))
	}
}

// ParseExpr parses a single expression (prop values, when, repeat.over, …).
func ParseExpr(src string) (Expr, []Diag) {
	p := newParser(src)
	var e Expr
	if !p.run(func() { e = p.expr(); p.endOfInput() }) {
		e = nil
	}
	return e, *p.diags
}

// ParseStmts parses a statement list (on.* values).
func ParseStmts(src string) ([]Stmt, []Diag) {
	p := newParser(src)
	var ss []Stmt
	p.run(func() { ss = p.stmtList("") })
	return ss, *p.diags
}

// ParseType parses a DSL type (contract signatures: `any` is allowed).
func ParseType(src string) (*Type, error) {
	p := newParser(src)
	var t *Type
	p.run(func() { t = p.typ(); p.endOfInput() })
	if len(*p.diags) > 0 {
		return nil, (*p.diags)[0]
	}
	return t, nil
}

// ParseScript parses the top-level declarations, recovering at the next line starting with a declaration keyword.
func ParseScript(src string) (*Script, []Diag) {
	p := newParser(src)
	p.script = true
	s := &Script{}
	for !p.atEOF() {
		start := p.i
		var d Decl
		if p.run(func() { d = p.decl() }) {
			s.Decls = append(s.Decls, d)
			if p.atEOF() || p.peek().nl || p.is(";") {
				for p.is(";") {
					p.next()
				}
				continue
			}
			p.errAt(p.peek().pos, "expected a newline or ';' before %s", desc(p.peek()))
		}
		if p.i == start {
			p.next()
		}
		for !p.atEOF() && !(p.peek().nl && p.peek().kind == tIdent && declKeywords[p.peek().text]) {
			p.next()
		}
	}
	return s, *p.diags
}

// --- expressions ---

func (p *parser) expr() Expr { return p.binary(1) }

func (p *parser) binary(min int) Expr {
	x := p.unary()
	chain := 0 // each operator nests the tree one level deeper
	defer func() { p.depth -= chain }()
	for {
		t := p.peek()
		if t.kind != tPunct {
			return x
		}
		if t.text == "?" {
			if min > 1 {
				return x
			}
			chain++
			p.nest(t.pos)
			p.next()
			a := p.binary(1)
			p.expect(":")
			x = &Cond{exprBase: at(x.Pos()), C: x, A: a, B: p.binary(1)}
			continue
		}
		prec, ok := binPrec[t.text]
		if !ok || prec < min {
			return x
		}
		chain++
		p.nest(t.pos)
		p.next()
		next := prec + 1
		if t.text == "??" {
			next = prec // right-associative
		}
		x = &Binary{exprBase: at(t.pos), Op: t.text, X: x, Y: p.binary(next)}
	}
}

func (p *parser) unary() Expr {
	defer func() { p.depth-- }()
	p.nest(p.peek().pos)
	if t := p.peek(); p.is("!") || p.is("-") {
		p.next()
		return &Unary{exprBase: at(t.pos), Op: t.text, X: p.unary()}
	}
	return p.postfix(p.primary())
}

// postfix: '.' may continue on a new line (method chains); '(' and '[' may not.
func (p *parser) postfix(x Expr) Expr {
	chain := 0 // each step nests the tree one level deeper
	defer func() { p.depth -= chain }()
	for {
		t := p.peek()
		if p.is(".") || (p.is("(") || p.is("[")) && !t.nl {
			chain++
			p.nest(t.pos)
		}
		switch {
		case p.is("."):
			p.next()
			n := p.peek()
			if n.kind != tIdent {
				p.fail(n.pos, "expected a field name after '.', found %s", desc(n))
			}
			p.next()
			x = &Member{exprBase: at(n.pos), X: x, Name: n.text}
		case p.is("(") && !t.nl:
			p.next()
			x = &Call{exprBase: at(x.Pos()), Fn: x, Args: p.exprList(")")}
		case p.is("[") && !t.nl:
			p.next()
			i := p.expr()
			p.expect("]")
			x = &Index{exprBase: at(t.pos), X: x, I: i}
		default:
			return x
		}
	}
}

func (p *parser) exprList(close string) []Expr {
	var es []Expr
	for !p.is(close) {
		es = append(es, p.expr())
		if !p.is(",") {
			break
		}
		p.next()
	}
	p.expect(close)
	return es
}

func (p *parser) primary() Expr {
	t := p.peek()
	switch t.kind {
	case tNum:
		p.next()
		return &Lit{exprBase: at(t.pos), Val: t.num}
	case tStr:
		p.next()
		return p.strExpr(t)
	case tTok:
		p.next()
		g, n, _ := strings.Cut(t.text, ".")
		return &TokenRef{exprBase: at(t.pos), Group: g, Name: n}
	case tIdent:
		switch t.text {
		case "true", "false":
			p.next()
			return &Lit{exprBase: at(t.pos), Val: t.text == "true"}
		case "null":
			p.next()
			return &Lit{exprBase: at(t.pos), Val: nil}
		case "event":
		default:
			if keywords[t.text] {
				p.fail(t.pos, "unexpected keyword '%s'", t.text)
			}
		}
		p.next()
		if p.is("=>") && t.text != "event" {
			p.next()
			return &Lambda{exprBase: at(t.pos), Params: []string{t.text}, Body: p.expr()}
		}
		return &Ident{exprBase: at(t.pos), Name: t.text}
	case tPunct:
		switch t.text {
		case "(":
			if params, ok := p.lambdaHead(); ok {
				return &Lambda{exprBase: at(t.pos), Params: params, Body: p.expr()}
			}
			p.next()
			x := p.expr()
			p.expect(")")
			return x
		case "[":
			p.next()
			return &ListLit{exprBase: at(t.pos), Elems: p.exprList("]")}
		case "{":
			return p.record()
		}
	}
	p.fail(t.pos, "unexpected %s", desc(t))
	return nil
}

// lambdaHead consumes `(a, b) =>` when the parenthesis starts a lambda.
func (p *parser) lambdaHead() ([]string, bool) {
	j := p.i + 1
	names := []string{}
	if !(p.toks[j].kind == tPunct && p.toks[j].text == ")") {
		for {
			t := p.toks[j]
			if t.kind != tIdent || keywords[t.text] {
				return nil, false
			}
			names = append(names, t.text)
			j++
			if p.toks[j].kind == tPunct && p.toks[j].text == "," {
				j++
				continue
			}
			break
		}
	}
	if p.toks[j].kind == tPunct && p.toks[j].text == ")" && p.toks[j+1].kind == tPunct && p.toks[j+1].text == "=>" {
		p.i = j + 2
		return names, true
	}
	return nil, false
}

func (p *parser) record() Expr {
	t := p.next()
	r := &RecLit{exprBase: at(t.pos)}
	for !p.is("}") {
		k := p.peek()
		key, ok := k.text, k.kind == tIdent
		if k.kind == tStr {
			key, ok = plainString(k)
		}
		if !ok {
			p.fail(k.pos, "expected a field name, found %s", desc(k))
		}
		p.next()
		p.expect(":")
		r.Fields = append(r.Fields, RecField{Name: key, P: k.pos, X: p.expr()})
		if !p.is(",") {
			break
		}
		p.next()
	}
	p.expect("}")
	return r
}

func plainString(t token) (string, bool) {
	switch {
	case len(t.parts) == 0:
		return "", true
	case len(t.parts) == 1 && t.parts[0].toks == nil:
		return t.parts[0].lit, true
	}
	return "", false
}

func (p *parser) strExpr(t token) Expr {
	if s, ok := plainString(t); ok {
		return &Lit{exprBase: at(t.pos), Val: s}
	}
	s := &Interp{exprBase: at(t.pos)}
	for _, part := range t.parts {
		if part.toks == nil {
			s.Parts = append(s.Parts, &Lit{exprBase: at(part.pos), Val: part.lit})
			continue
		}
		sub := &parser{toks: part.toks, diags: p.diags, depth: p.depth}
		x := sub.expr()
		sub.endOfInput()
		s.Parts = append(s.Parts, x)
	}
	return s
}

// --- statements ---

func (p *parser) stmtList(close string) []Stmt {
	var ss []Stmt
	for {
		for p.is(";") {
			p.next()
		}
		if p.atEOF() || close != "" && p.is(close) {
			return ss
		}
		ss = append(ss, p.stmt())
		if !p.atEOF() && !p.is(";") && !(close != "" && p.is(close)) && !p.peek().nl {
			p.fail(p.peek().pos, "expected a newline or ';' before %s", desc(p.peek()))
		}
	}
}

func (p *parser) block() []Stmt {
	defer func() { p.depth-- }()
	p.nest(p.peek().pos)
	p.expect("{")
	ss := p.stmtList("}")
	p.expect("}")
	return ss
}

func (p *parser) stmt() Stmt {
	t := p.peek()
	isCall := func(k int) bool {
		return p.peekN(k).kind == tIdent && p.peekN(k).text == "call" && p.peekN(k+1).kind == tIdent
	}
	if t.kind == tIdent {
		switch t.text {
		case "let":
			p.next()
			name, _ := p.name()
			p.expect("=")
			if p.isKw("await") && isCall(1) {
				p.next()
				p.next()
				cs := p.opCall(t.pos)
				cs.Await, cs.Result = true, name
				return cs
			}
			if isCall(0) {
				p.fail(p.peek().pos, "a call result can only be bound with 'let %s = await call …'", name)
			}
			return &LetStmt{P: t.pos, Name: name, Value: p.expr()}
		case "if":
			return p.ifStmt()
		case "animate":
			p.next()
			p.expect("(")
			m := p.expr()
			p.expect(")")
			return &AnimStmt{P: t.pos, Motion: m, Body: p.block()}
		case "await":
			p.next()
			if isCall(0) {
				p.next()
				cs := p.opCall(t.pos)
				cs.Await = true
				return cs
			}
			c, ok := p.expr().(*Call)
			if !ok {
				p.fail(t.pos, "expected a call after 'await'")
			}
			return &ExprStmt{P: t.pos, Call: c, Await: true}
		case "emit":
			p.next()
			name, _ := p.name()
			return &EmitStmt{P: t.pos, Name: name}
		case "call":
			if isCall(0) {
				p.next()
				return p.opCall(t.pos)
			}
		}
	}
	x := p.expr()
	if p.is("=") {
		p.next()
		return &Assign{P: t.pos, Target: x, Value: p.expr()}
	}
	c, ok := x.(*Call)
	if !ok {
		p.fail(t.pos, "expected a statement (assignment, call, let, if, animate, emit)")
	}
	return &ExprStmt{P: t.pos, Call: c}
}

func (p *parser) ifStmt() *IfStmt {
	defer func() { p.depth-- }()
	p.nest(p.peek().pos)
	t := p.next()
	s := &IfStmt{P: t.pos, Cond: p.expr()}
	s.Then = p.block()
	if p.isKw("else") {
		p.next()
		if p.isKw("if") {
			s.Else = []Stmt{p.ifStmt()}
		} else {
			s.Else = p.block()
		}
	}
	return s
}

// opCall parses `demo.checkPromo(args?)`.
func (p *parser) opCall(pos Pos) *CallStmt {
	cs := &CallStmt{P: pos, OpPos: p.peek().pos}
	for {
		t := p.peek()
		if t.kind != tIdent {
			p.fail(t.pos, "expected an operation id, found %s", desc(t))
		}
		p.next()
		cs.Op += t.text
		if !p.is(".") {
			break
		}
		p.next()
		cs.Op += "."
	}
	p.expect("(")
	if !p.is(")") {
		cs.Args = p.expr()
	}
	p.expect(")")
	return cs
}

// --- declarations ---

func (p *parser) decl() Decl {
	t := p.peek()
	if t.kind == tIdent {
		switch t.text {
		case "state", "let":
			p.next()
			name, np := p.name()
			var ann *Type
			if p.is(":") {
				p.next()
				ann = p.typ()
			}
			p.expect("=")
			if t.text == "state" {
				return &StateDecl{P: np, Name: name, Ann: ann, Init: p.expr()}
			}
			return &LetDecl{P: np, Name: name, Ann: ann, Value: p.expr()}
		case "fn":
			p.next()
			name, np := p.name()
			d := &FnDecl{P: np, Name: name, Params: p.params()}
			if p.is(":") {
				p.next()
				d.Ret = p.typ()
			}
			p.expect("=")
			d.Body = p.expr()
			return d
		case "handler":
			p.next()
			name, np := p.name()
			d := &HandlerDecl{P: np, Name: name, Params: p.params()}
			d.Body = p.block()
			return d
		case "data":
			p.next()
			name, np := p.name()
			p.expect("=")
			cs := p.opCall(p.peek().pos)
			d := &DataDecl{P: np, Name: name, Op: cs.Op, OpPos: cs.OpPos, Args: cs.Args}
			if p.isKw("required") && !p.peek().nl {
				p.next()
				d.Required = true
			}
			return d
		case "param":
			p.next()
			name, np := p.name()
			p.expect(":")
			d := &ParamDecl{P: np, Name: name, T: p.typ()}
			if p.is("=") {
				p.next()
				d.Default = p.expr()
			}
			return d
		case "event":
			p.next()
			name, np := p.name()
			return &EventDecl{P: np, Name: name}
		}
	}
	p.fail(t.pos, "expected a declaration (state, let, fn, handler, data, param, event), found %s", desc(t))
	return nil
}

func (p *parser) params() []ParamDef {
	p.expect("(")
	var ps []ParamDef
	for !p.is(")") {
		name, np := p.name()
		p.expect(":")
		ps = append(ps, ParamDef{Name: name, P: np, T: p.typ()})
		if !p.is(",") {
			break
		}
		p.next()
	}
	p.expect(")")
	return ps
}

// --- types ---

func (p *parser) typ() *Type {
	alts := []*Type{p.typOpt()}
	for p.is("|") {
		p.next()
		alts = append(alts, p.typOpt())
	}
	if len(alts) == 1 {
		return alts[0]
	}
	return UnionOf(alts...)
}

func (p *parser) typOpt() *Type {
	t := p.typPrim()
	for p.is("?") {
		p.next()
		t = OptOf(t)
	}
	return t
}

func (p *parser) typPrim() *Type {
	t := p.peek()
	switch {
	case t.kind == tStr:
		s, ok := plainString(t)
		if !ok {
			p.fail(t.pos, "interpolation is not allowed in a type")
		}
		p.next()
		return LitOf(s)
	case p.is("{"):
		p.next()
		r := &Type{Kind: KRecord}
		for !p.is("}") {
			ft := p.peek()
			if ft.kind != tIdent {
				p.fail(ft.pos, "expected a field name, found %s", desc(ft))
			}
			p.next()
			opt := p.is("?")
			if opt {
				p.next()
			}
			p.expect(":")
			if r.field(ft.text) != nil {
				p.fail(ft.pos, "duplicate field '%s'", ft.text)
			}
			r.Fields = append(r.Fields, Field{Name: ft.text, T: p.typ(), Opt: opt})
			if !p.is(",") {
				break
			}
			p.next()
		}
		p.expect("}")
		return r
	case p.is("("):
		p.next()
		var ts []*Type
		for !p.is(")") {
			ts = append(ts, p.typ())
			if !p.is(",") {
				break
			}
			p.next()
		}
		p.expect(")")
		if p.sig && p.is("=>") {
			p.next()
			return &Type{Kind: KFunc, Params: ts, Ret: p.typ()}
		}
		if len(ts) != 1 {
			p.fail(t.pos, "expected a type")
		}
		return ts[0]
	case p.sig && p.is("<"):
		// contract placeholder such as <data name>, <timeline name>, <op result>
		p.next()
		var words []string
		for !p.is(">") && !p.atEOF() {
			words = append(words, p.next().text)
		}
		p.expect(">")
		return &Type{Kind: KVar, Name: strings.Join(words, " ")}
	case t.kind == tIdent:
		p.next()
		if p.sig && p.vars[t.text] {
			return &Type{Kind: KVar, Name: t.text}
		}
		switch t.text {
		case "list", "map":
			p.expect("<")
			e := p.typ()
			p.closeAngle()
			if t.text == "list" {
				return ListOf(e)
			}
			return MapOf(e)
		case "any":
			if p.script {
				p.errAt(t.pos, "type 'any' is only allowed in contract signatures")
			}
		}
		if b, ok := basicTypes[t.text]; ok {
			return b
		}
		p.fail(t.pos, "unknown type '%s'", t.text)
	}
	p.fail(t.pos, "expected a type, found %s", desc(t))
	return nil
}

// closeAngle accepts '>' and splits '>=' (as in `list<number>= []`).
func (p *parser) closeAngle() {
	if p.is(">=") {
		p.toks[p.i].text = "="
		p.toks[p.i].pos.Col++
		return
	}
	p.expect(">")
}
