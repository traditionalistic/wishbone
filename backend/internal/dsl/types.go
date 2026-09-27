package dsl

import (
	"strconv"
	"strings"
)

type Kind uint8

const (
	KBool Kind = iota
	KNumber
	KString
	KColor
	KPaint
	KTextStyle
	KMotion
	KIcon
	KNull
	KAny
	KLit    // string literal type, Name = value
	KList   // Elem
	KMap    // Elem, string keys
	KOpt    // Elem?
	KRecord // Fields
	KUnion  // Alts (never contains null or optionals: those are hoisted into KOpt)
	KFunc   // Params, Ret — lambdas and stdlib signatures only
	KVar    // stdlib generic parameter or <placeholder>, Name
)

// Type is immutable once built; build with the constructors so optionals and unions stay normalized.
type Type struct {
	Kind   Kind
	Name   string
	Elem   *Type
	Fields []Field
	Alts   []*Type
	Params []*Type
	Ret    *Type
}

type Field struct {
	Name string
	T    *Type
	Opt  bool
}

var basicNames = [...]string{KBool: "bool", KNumber: "number", KString: "string", KColor: "color", KPaint: "paint",
	KTextStyle: "textStyle", KMotion: "motion", KIcon: "icon", KNull: "null", KAny: "any"}

var (
	TBool      = &Type{Kind: KBool}
	TNumber    = &Type{Kind: KNumber}
	TString    = &Type{Kind: KString}
	TColor     = &Type{Kind: KColor}
	TPaint     = &Type{Kind: KPaint}
	TTextStyle = &Type{Kind: KTextStyle}
	TMotion    = &Type{Kind: KMotion}
	TIcon      = &Type{Kind: KIcon}
	TNull      = &Type{Kind: KNull}
	TAny       = &Type{Kind: KAny}
)

var basicTypes = map[string]*Type{"bool": TBool, "number": TNumber, "string": TString, "color": TColor, "paint": TPaint,
	"textStyle": TTextStyle, "motion": TMotion, "icon": TIcon, "null": TNull, "any": TAny}

func ListOf(t *Type) *Type { return &Type{Kind: KList, Elem: t} }
func MapOf(t *Type) *Type  { return &Type{Kind: KMap, Elem: t} }
func LitOf(s string) *Type { return &Type{Kind: KLit, Name: s} }

// OptOf returns t? with (T?)? = T?, null? = null, any? = any.
func OptOf(t *Type) *Type {
	if t == nil {
		return nil
	}
	switch t.Kind {
	case KOpt, KNull, KAny:
		return t
	}
	return &Type{Kind: KOpt, Elem: t}
}

// UnionOf flattens, dedupes and hoists null/optionals: null | A? | B = (A | B)?.
func UnionOf(ts ...*Type) *Type {
	var alts []*Type
	opt := false
	var add func(t *Type)
	add = func(t *Type) {
		switch t.Kind {
		case KUnion:
			for _, a := range t.Alts {
				add(a)
			}
			return
		case KOpt:
			opt = true
			add(t.Elem)
			return
		case KNull:
			opt = true
			return
		}
		for _, a := range alts {
			if Equal(a, t) {
				return
			}
		}
		alts = append(alts, t)
	}
	for _, t := range ts {
		add(t)
	}
	var r *Type
	switch len(alts) {
	case 0:
		return TNull
	case 1:
		r = alts[0]
	default:
		r = &Type{Kind: KUnion, Alts: alts}
	}
	if opt {
		return OptOf(r)
	}
	return r
}

func (t *Type) field(name string) *Field {
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return &t.Fields[i]
		}
	}
	return nil
}

func nonOpt(t *Type) *Type {
	if t != nil && t.Kind == KOpt {
		return t.Elem
	}
	return t
}

func (t *Type) String() string {
	if t == nil {
		return "<error>"
	}
	switch t.Kind {
	case KLit:
		return strconv.Quote(t.Name)
	case KList:
		return "list<" + t.Elem.String() + ">"
	case KMap:
		return "map<" + t.Elem.String() + ">"
	case KOpt:
		if t.Elem.Kind == KUnion || t.Elem.Kind == KFunc {
			return "(" + t.Elem.String() + ")?"
		}
		return t.Elem.String() + "?"
	case KRecord:
		fs := make([]string, len(t.Fields))
		for i, f := range t.Fields {
			q := ""
			if f.Opt {
				q = "?"
			}
			fs[i] = f.Name + q + ": " + f.T.String()
		}
		return "{" + strings.Join(fs, ", ") + "}"
	case KUnion:
		as := make([]string, len(t.Alts))
		for i, a := range t.Alts {
			as[i] = a.String()
			if a.Kind == KFunc {
				as[i] = "(" + as[i] + ")"
			}
		}
		return strings.Join(as, " | ")
	case KFunc:
		ps := make([]string, len(t.Params))
		for i, p := range t.Params {
			ps[i] = p.String()
		}
		return "(" + strings.Join(ps, ", ") + ") => " + t.Ret.String()
	case KVar:
		if strings.Contains(t.Name, " ") {
			return "<" + t.Name + ">"
		}
		return t.Name
	}
	return basicNames[t.Kind]
}

// Assignable reports whether a value of type src may be used where dst is expected.
// A nil type is the checker's "already reported" error type and is assignable both ways.
func Assignable(src, dst *Type) bool {
	if src == nil || dst == nil {
		return true
	}
	if dst.Kind == KAny {
		return src.Kind != KFunc
	}
	switch src.Kind {
	case KUnion:
		for _, a := range src.Alts {
			if !Assignable(a, dst) {
				return false
			}
		}
		return true
	case KOpt:
		return dst.Kind == KOpt && Assignable(src.Elem, dst.Elem)
	case KNull:
		return dst.Kind == KOpt || dst.Kind == KNull
	}
	switch dst.Kind {
	case KOpt:
		return Assignable(src, dst.Elem)
	case KUnion:
		for _, a := range dst.Alts {
			if Assignable(src, a) {
				return true
			}
		}
		return false
	case KString:
		return src.Kind == KString || src.Kind == KLit
	case KLit:
		return src.Kind == KLit && src.Name == dst.Name
	case KPaint:
		return src.Kind == KPaint || src.Kind == KColor
	case KList:
		return src.Kind == KList && Assignable(src.Elem, dst.Elem)
	case KMap:
		switch src.Kind {
		case KMap:
			return Assignable(src.Elem, dst.Elem)
		case KRecord:
			for _, f := range src.Fields {
				if !Assignable(f.T, dst.Elem) {
					return false
				}
			}
			return true
		}
		return false
	case KRecord:
		return src.Kind == KRecord && whyNot(src, dst) == ""
	case KFunc:
		return false
	case KVar:
		return src.Kind == KVar && src.Name == dst.Name
	}
	return src.Kind == dst.Kind
}

// whyNot explains why record src is not assignable to record dst ("" = it is).
func whyNot(src, dst *Type) string {
	if src.Kind != KRecord || dst.Kind != KRecord {
		if Assignable(src, dst) {
			return ""
		}
		return "cannot use " + src.String() + " as " + dst.String()
	}
	for _, f := range src.Fields {
		if dst.field(f.Name) == nil {
			return "unknown field '" + f.Name + "'"
		}
	}
	for _, df := range dst.Fields {
		sf := src.field(df.Name)
		switch {
		case sf == nil && !df.Opt:
			return "missing field '" + df.Name + "'"
		case sf == nil:
		case sf.Opt && !df.Opt:
			return "field '" + df.Name + "' is required"
		case !Assignable(sf.T, df.T):
			return "field '" + df.Name + "': " + whyNot(sf.T, df.T)
		}
	}
	return ""
}

func Equal(a, b *Type) bool { return Assignable(a, b) && Assignable(b, a) }

// join is the least common type of two branches / list elements, or nil if there is none.
func join(a, b *Type) *Type {
	switch {
	case Assignable(a, b):
		return b
	case Assignable(b, a):
		return a
	case a.Kind == KNull:
		return OptOf(b)
	case b.Kind == KNull:
		return OptOf(a)
	case a.Kind == KOpt || b.Kind == KOpt:
		if j := join(nonOpt(a), nonOpt(b)); j != nil {
			return OptOf(j)
		}
	}
	return nil
}

func hasVar(t *Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case KVar:
		return true
	case KList, KMap, KOpt:
		return hasVar(t.Elem)
	case KRecord:
		for _, f := range t.Fields {
			if hasVar(f.T) {
				return true
			}
		}
	case KUnion:
		for _, a := range t.Alts {
			if hasVar(a) {
				return true
			}
		}
	case KFunc:
		for _, p := range t.Params {
			if hasVar(p) {
				return true
			}
		}
		return hasVar(t.Ret)
	}
	return false
}

func subst(t *Type, b map[string]*Type) *Type {
	if !hasVar(t) {
		return t
	}
	switch t.Kind {
	case KVar:
		if r, ok := b[t.Name]; ok {
			return r
		}
		return t
	case KList:
		return ListOf(subst(t.Elem, b))
	case KMap:
		return MapOf(subst(t.Elem, b))
	case KOpt:
		return OptOf(subst(t.Elem, b))
	case KRecord:
		r := &Type{Kind: KRecord, Fields: make([]Field, len(t.Fields))}
		for i, f := range t.Fields {
			r.Fields[i] = Field{f.Name, subst(f.T, b), f.Opt}
		}
		return r
	case KUnion:
		as := make([]*Type, len(t.Alts))
		for i, a := range t.Alts {
			as[i] = subst(a, b)
		}
		return UnionOf(as...)
	case KFunc:
		r := &Type{Kind: KFunc, Params: make([]*Type, len(t.Params)), Ret: subst(t.Ret, b)}
		for i, p := range t.Params {
			r.Params[i] = subst(p, b)
		}
		return r
	}
	return t
}

// closed returns subst(t, b) when it has no unbound variables, else nil (no hint).
func closed(t *Type, b map[string]*Type) *Type {
	if r := subst(t, b); !hasVar(r) {
		return r
	}
	return nil
}

// unify matches an actual argument type against a signature pattern, binding variables in b.
func unify(p, a *Type, b map[string]*Type) bool {
	if !hasVar(p) {
		return Assignable(a, p)
	}
	if a == nil {
		return true
	}
	switch p.Kind {
	case KVar:
		if bt, ok := b[p.Name]; ok {
			return Assignable(a, bt)
		}
		b[p.Name] = a
		return true
	case KList, KMap:
		return a.Kind == p.Kind && unify(p.Elem, a.Elem, b)
	case KOpt:
		if a.Kind == KNull {
			return true
		}
		return unify(p.Elem, nonOpt(a), b)
	}
	return false
}

// hasLit reports whether want admits string literal types (so a literal keeps its literal type).
func hasLit(want *Type) bool {
	want = nonOpt(want)
	if want == nil {
		return false
	}
	if want.Kind == KLit {
		return true
	}
	if want.Kind == KUnion {
		for _, a := range want.Alts {
			if a.Kind == KLit {
				return true
			}
		}
	}
	return false
}

// pick returns the alternative of want (after stripping ?) with the given kind, or nil.
func pick(want *Type, kinds ...Kind) *Type {
	want = nonOpt(want)
	if want == nil {
		return nil
	}
	alts := []*Type{want}
	if want.Kind == KUnion {
		alts = want.Alts
	}
	for _, a := range alts {
		for _, k := range kinds {
			if a.Kind == k {
				return a
			}
		}
	}
	return nil
}

func isKind(t *Type, k Kind) bool { return t != nil && t.Kind == k }
func stringy(t *Type) bool        { return t != nil && t.Kind != KNull && Assignable(t, TString) }
