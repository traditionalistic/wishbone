package dsl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Stdlib is contract/stdlib.json with signatures parsed.
type Stdlib struct {
	Funcs      map[string]*Func
	Effects    map[string]*Func
	Env        map[string]*Type
	CallResult *Type // record type containing the placeholder <op result>
	Easings    map[string][4]float64
}

// Func is a stdlib function or effect.
type Func struct {
	Name             string
	Sigs             []*Sig // overloads: the first accepting the arguments wins
	Method, Property bool
	Special          bool // checked by a dedicated rule (see specialFuncs)
}

type Sig struct {
	Vars   []string
	Params []SigParam
	Ret    *Type // nil for effects
}

type SigParam struct {
	Name string
	T    *Type
	Opt  bool
}

// Op is an operation from contract/operations.json.
type Op struct {
	ID, Kind     string // Kind: "read" | "write"
	Args, Result *Type
	Timeout      time.Duration // data-source timeout; 0 = the server default
}

var specialFuncs = map[string]bool{"with": true}

// Lambdas passed to these may omit the trailing index parameter.
var indexLambdas = map[string]bool{"map": true, "filter": true, "find": true, "some": true, "every": true}

func LoadStdlib(data []byte) (*Stdlib, error) {
	var raw struct {
		Functions map[string]struct {
			Sig                       json.RawMessage
			Method, Property, Special bool
		}
		Effects    map[string]struct{ Sig string }
		Env        map[string]string
		CallResult string
		Easings    map[string][4]float64
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	s := &Stdlib{Funcs: map[string]*Func{}, Effects: map[string]*Func{}, Env: map[string]*Type{}, Easings: raw.Easings}
	for name, f := range raw.Functions {
		var srcs []string
		if err := json.Unmarshal(f.Sig, &srcs); err != nil {
			var one string
			if err := json.Unmarshal(f.Sig, &one); err != nil {
				return nil, fmt.Errorf("stdlib %s: sig must be a string or a list of strings", name)
			}
			srcs = []string{one}
		}
		if f.Special && !specialFuncs[name] {
			return nil, fmt.Errorf("stdlib %s: special function without a checker rule", name)
		}
		fn := &Func{Name: name, Method: f.Method, Property: f.Property, Special: f.Special}
		for _, src := range srcs {
			sig, err := parseSig(src)
			if err != nil {
				return nil, fmt.Errorf("stdlib %s: %v", name, err)
			}
			fn.Sigs = append(fn.Sigs, sig)
		}
		s.Funcs[name] = fn
	}
	for name, e := range raw.Effects {
		sig, err := parseSig(e.Sig)
		if err != nil {
			return nil, fmt.Errorf("stdlib effect %s: %v", name, err)
		}
		s.Effects[name] = &Func{Name: name, Sigs: []*Sig{sig}}
	}
	for name, src := range raw.Env {
		t, err := ParseType(src)
		if err != nil {
			return nil, fmt.Errorf("stdlib env.%s: %v", name, err)
		}
		s.Env[name] = t
	}
	if raw.CallResult != "" {
		p := newParser(raw.CallResult)
		p.sig = true
		p.run(func() { s.CallResult = p.typ(); p.endOfInput() })
		if len(*p.diags) > 0 {
			return nil, fmt.Errorf("stdlib callResult: %v", (*p.diags)[0])
		}
	}
	return s, nil
}

func LoadOps(data []byte) (map[string]*Op, error) {
	var raw struct {
		Operations map[string]struct {
			Kind, Args, Result string
			TimeoutMs          int
		}
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	ops := map[string]*Op{}
	for id, o := range raw.Operations {
		args, err := ParseType(o.Args)
		if err != nil {
			return nil, fmt.Errorf("operation %s args: %v", id, err)
		}
		res, err := ParseType(o.Result)
		if err != nil {
			return nil, fmt.Errorf("operation %s result: %v", id, err)
		}
		ops[id] = &Op{ID: id, Kind: o.Kind, Args: args, Result: res, Timeout: time.Duration(o.TimeoutMs) * time.Millisecond}
	}
	return ops, nil
}

// parseSig parses `<T, U>(name: Type, opt?: Type) => Ret`.
func parseSig(src string) (*Sig, error) {
	p := newParser(src)
	p.sig, p.vars = true, map[string]bool{}
	sig := &Sig{}
	p.run(func() {
		if p.is("<") {
			p.next()
			for {
				t := p.next()
				p.vars[t.text] = true
				sig.Vars = append(sig.Vars, t.text)
				if !p.is(",") {
					break
				}
				p.next()
			}
			p.expect(">")
		}
		p.expect("(")
		for !p.is(")") {
			t := p.next()
			if t.kind != tIdent {
				p.fail(t.pos, "expected a parameter name")
			}
			opt := p.is("?")
			if opt {
				p.next()
			}
			p.expect(":")
			sig.Params = append(sig.Params, SigParam{Name: t.text, T: p.typ(), Opt: opt})
			if !p.is(",") {
				break
			}
			p.next()
		}
		p.expect(")")
		if p.is("=>") {
			p.next()
			sig.Ret = p.typ()
		}
		p.endOfInput()
	})
	if len(*p.diags) > 0 {
		return nil, (*p.diags)[0]
	}
	return sig, nil
}

func (s *Sig) String() string {
	var b strings.Builder
	if len(s.Vars) > 0 {
		b.WriteString("<" + strings.Join(s.Vars, ", ") + ">")
	}
	ps := make([]string, len(s.Params))
	for i, p := range s.Params {
		q := ""
		if p.Opt {
			q = "?"
		}
		ps[i] = p.Name + q + ": " + p.T.String()
	}
	b.WriteString("(" + strings.Join(ps, ", ") + ")")
	if s.Ret != nil {
		b.WriteString(" => " + s.Ret.String())
	}
	return b.String()
}

func (f *Func) String() string {
	ss := make([]string, len(f.Sigs))
	for i, s := range f.Sigs {
		ss[i] = s.String()
	}
	return strings.Join(ss, " | ")
}

func arity(min, max int) string {
	if min == max {
		return strconv.Itoa(min)
	}
	return fmt.Sprintf("%d to %d", min, max)
}

// cannotUse is the standard mismatch message, with the reason for records.
func cannotUse(src, dst *Type) string {
	msg := "cannot use " + src.String() + " as " + dst.String()
	if r := pick(dst, KRecord); src.Kind == KRecord && r != nil {
		if why := whyNot(src, r); why != "" {
			msg += ": " + why
		}
	}
	return msg
}

// stdCall checks a stdlib call. When recvT != nil, args[0] is a receiver already checked (and made non-optional).
func (c *ctx) stdCall(pos Pos, f *Func, args []Expr, recvT *Type) *Type {
	if f.Special {
		return c.withCall(pos, args, recvT)
	}
	if len(f.Sigs) == 1 {
		t, _ := c.sigCall(pos, f, f.Sigs[0], args, recvT)
		return t
	}
	if recvT == nil && len(args) > 0 { // check the first argument once, not once per overload (nesting was exponential)
		if recvT = c.expr(args[0], nil); recvT == nil {
			return nil
		}
	}
	var best []Diag
	for _, sig := range f.Sigs {
		var scratch []Diag
		trial := *c
		trial.diags = &scratch
		t, firstOK := trial.sigCall(pos, f, sig, args, recvT)
		if len(scratch) == 0 {
			return t
		}
		if firstOK && best == nil {
			best = scratch
		}
	}
	if best != nil { // the overload chosen by the first argument explains the error best
		*c.diags = append(*c.diags, best...)
		return nil
	}
	first := recvT
	if first == nil && len(args) > 0 {
		first = args[0].Type()
	}
	c.typeErr(pos, "no overload of %s accepts %s (overloads: %s)", f.Name, first, f)
	return nil
}

// sigCall checks args against one signature; firstOK reports that the first argument matched.
func (c *ctx) sigCall(pos Pos, f *Func, sig *Sig, args []Expr, recvT *Type) (t *Type, firstOK bool) {
	min := 0
	for _, p := range sig.Params {
		if !p.Opt {
			min++
		}
	}
	if len(args) < min || len(args) > len(sig.Params) {
		c.typeErr(pos, "wrong number of arguments for %s: want %s, got %d", f.Name, arity(min, len(sig.Params)), len(args))
		return nil, false
	}
	b := map[string]*Type{}
	for i, a := range args {
		pt := sig.Params[i].T
		if lam, ok := a.(*Lambda); ok && pt.Kind == KFunc {
			if !c.lambda(lam, f, pt, b) {
				return nil, firstOK
			}
			continue
		}
		ty := recvT
		if i > 0 || recvT == nil {
			ty = c.expr(a, closed(pt, b))
		}
		if ty == nil {
			return nil, firstOK
		}
		if !unify(pt, ty, b) {
			c.typeErr(a.Pos(), "argument %d of %s: %s", i+1, f.Name, cannotUse(ty, subst(pt, b)))
			return nil, firstOK
		}
		if i == 0 {
			firstOK = true
		}
	}
	return subst(sig.Ret, b), firstOK
}

func (c *ctx) lambda(lam *Lambda, f *Func, ft *Type, b map[string]*Type) bool {
	n, min := len(ft.Params), len(ft.Params)
	if indexLambdas[f.Name] {
		min = 1
	}
	if len(lam.Params) < min || len(lam.Params) > n {
		c.typeErr(lam.P, "lambda for %s takes %s parameters, got %d", f.Name, arity(min, n), len(lam.Params))
		return false
	}
	vars := map[string]*Type{}
	ps := make([]*Type, len(lam.Params))
	for i, name := range lam.Params {
		t := subst(ft.Params[i], b)
		if hasVar(t) {
			c.typeErr(lam.P, "cannot infer the type of lambda parameter '%s'", name)
			return false
		}
		if _, dup := vars[name]; dup {
			c.errf(lam.P, "duplicate lambda parameter '%s'", name)
			return false
		}
		vars[name], ps[i] = t, t
	}
	rt := c.push(vars).expr(lam.Body, closed(ft.Ret, b))
	if rt == nil {
		return false
	}
	if !unify(ft.Ret, rt, b) {
		c.typeErr(lam.Body.Pos(), "lambda for %s must return %s, got %s", f.Name, subst(ft.Ret, b), rt)
		return false
	}
	if ft.Ret.Kind == KUnion && !anyAlt(ft.Ret, rt) { // only sortBy has a union key type: all numbers or all strings
		c.typeErr(lam.Body.Pos(), "%s keys must be all numbers or all strings, got %s", f.Name, rt)
		return false
	}
	lam.T = &Type{Kind: KFunc, Params: ps, Ret: rt}
	return true
}

func anyAlt(u, t *Type) bool {
	for _, a := range u.Alts {
		if Assignable(t, a) {
			return true
		}
	}
	return false
}

// withCall: r.with({…}) — the patch is a record literal naming a subset of r's fields.
func (c *ctx) withCall(pos Pos, args []Expr, recvT *Type) *Type {
	if len(args) != 2 {
		c.typeErr(pos, "wrong number of arguments for with: want 2, got %d", len(args))
		return nil
	}
	r := recvT
	if r == nil {
		r = c.expr(args[0], nil)
	}
	if r == nil {
		return nil
	}
	if r.Kind != KRecord {
		c.typeErr(args[0].Pos(), "with needs a record, got %s", r)
		return nil
	}
	lit, ok := args[1].(*RecLit)
	if !ok {
		c.typeErr(args[1].Pos(), "the patch of with must be a record literal")
		return nil
	}
	patch := &Type{Kind: KRecord, Fields: make([]Field, len(r.Fields))}
	for i, f := range r.Fields {
		patch.Fields[i] = Field{Name: f.Name, T: f.T, Opt: true}
	}
	if pt := c.expr(lit, patch); pt != nil && !Assignable(pt, patch) {
		c.typeErr(lit.P, "argument 2 of with: %s", cannotUse(pt, patch))
	}
	return r
}
