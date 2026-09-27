// Package dsl is the Wishbone DSL front end: lexer, parser, type checker and
// lowering to the wire AST (spec §3). It is the only parser in the system.
package dsl

import "fmt"

// Pos is a 1-based position inside the parsed source string (columns count runes).
type Pos struct{ Line, Col int }

// Diag is a compile diagnostic.
type Diag struct {
	Line, Col int
	Msg       string
}

func (d Diag) Error() string { return fmt.Sprintf("%d:%d: %s", d.Line, d.Col, d.Msg) }

// Expr is an expression node. After checking, Type() is its type (nil = error).
type Expr interface {
	Pos() Pos
	Type() *Type
	base() *exprBase
}

type exprBase struct {
	P Pos
	T *Type
}

func (b *exprBase) Pos() Pos        { return b.P }
func (b *exprBase) Type() *Type     { return b.T }
func (b *exprBase) base() *exprBase { return b }
func at(p Pos) exprBase             { return exprBase{P: p} }

// RefKind says what a resolved name refers to; it picks the wire form.
type RefKind uint8

const (
	RefNone     RefKind = iota
	RefState            // ["s", n]
	RefInst             // ["i", n] — template instance state
	RefComputed         // ["c", n]
	RefData             // ["d", n]
	RefLocal            // ["l", n] — let, handler/fn/lambda param, repeat var, event
	RefParam            // ["p", n]
	RefBare             // bare name argument of refresh/play/stop → ["v", n]
)

type MemberKind uint8

const (
	MemField  MemberKind = iota + 1 // ["get", x, name]
	MemProp                         // stdlib property → ["f", name, [x]]
	MemEnv                          // ["e", name]
	MemDriver                       // ["dr", kind, id, axis]
	MemSelf                         // ["self", "pressed"]
)

type CallKind uint8

const (
	CallStd     CallKind = iota + 1 // ["f", name, args]
	CallFn                          // ["u", name, args]
	CallHandler                     // statement: ["run", name, args]
	CallEffect                      // statement: ["do", name, args, await]
)

type (
	// Lit is a number (float64), string, bool or null (nil).
	Lit struct {
		exprBase
		Val any
	}
	// Interp is an interpolated string; literal chunks are *Lit strings.
	Interp struct {
		exprBase
		Parts []Expr
	}
	Ident struct {
		exprBase
		Name string
		Ref  RefKind
	}
	TokenRef struct {
		exprBase
		Group, Name string
	}
	Member struct {
		exprBase // P is the position of Name
		X        Expr
		Name     string
		Kind     MemberKind
	}
	Index struct {
		exprBase
		X, I Expr
	}
	Call struct {
		exprBase
		Fn   Expr
		Args []Expr
		Kind CallKind
		Name string // resolved callee
		Recv Expr   // method sugar receiver, passed as the first argument
	}
	Unary struct {
		exprBase
		Op string
		X  Expr
	}
	Binary struct {
		exprBase // P is the operator position
		Op       string
		X, Y     Expr
	}
	Cond struct {
		exprBase
		C, A, B Expr
	}
	ListLit struct {
		exprBase
		Elems []Expr
	}
	RecLit struct {
		exprBase
		Fields []RecField
	}
	Lambda struct {
		exprBase
		Params []string
		Body   Expr
	}
)

type RecField struct {
	Name string
	P    Pos
	X    Expr
}

// Stmt is a handler / on.* statement.
type Stmt interface{ Pos() Pos }

type (
	// Assign: Target is the parsed left side; the checker fills Root, Path (*Member / *Index segments) and Inst.
	Assign struct {
		P      Pos
		Target Expr
		Value  Expr
		Root   string
		Path   []Expr
		Inst   bool
	}
	LetStmt struct {
		P     Pos
		Name  string
		Value Expr
	}
	// CallStmt is `[let r = await] call op.id(args)`; Args is nil for `op.id()`.
	CallStmt struct {
		P      Pos
		Op     string
		OpPos  Pos
		Args   Expr
		Result string
		Await  bool
	}
	IfStmt struct {
		P          Pos
		Cond       Expr
		Then, Else []Stmt
	}
	AnimStmt struct {
		P      Pos
		Motion Expr
		Body   []Stmt
	}
	// ExprStmt is a handler run or an effect.
	ExprStmt struct {
		P     Pos
		Call  *Call
		Await bool
	}
	EmitStmt struct {
		P    Pos
		Name string
	}
)

func (s *Assign) Pos() Pos   { return s.P }
func (s *LetStmt) Pos() Pos  { return s.P }
func (s *CallStmt) Pos() Pos { return s.P }
func (s *IfStmt) Pos() Pos   { return s.P }
func (s *AnimStmt) Pos() Pos { return s.P }
func (s *ExprStmt) Pos() Pos { return s.P }
func (s *EmitStmt) Pos() Pos { return s.P }

// Script is the top-level `script` block.
type Script struct{ Decls []Decl }

// Decl is a top-level declaration; P is the position of its name.
type Decl interface {
	Pos() Pos
	DeclName() string
}

type (
	StateDecl struct {
		P    Pos
		Name string
		Ann  *Type // annotation or nil
		Init Expr
		T    *Type // declared or inferred, set by the checker
	}
	// LetDecl is a computed value.
	LetDecl struct {
		P     Pos
		Name  string
		Ann   *Type
		Value Expr
		T     *Type
	}
	FnDecl struct {
		P      Pos
		Name   string
		Params []ParamDef
		Ret    *Type // annotation, or inferred by the checker
		Body   Expr
	}
	HandlerDecl struct {
		P      Pos
		Name   string
		Params []ParamDef
		Body   []Stmt
	}
	DataDecl struct {
		P        Pos
		Name     string
		Op       string
		OpPos    Pos
		Args     Expr
		Required bool
		T        *Type
	}
	ParamDecl struct {
		P       Pos
		Name    string
		T       *Type
		Default Expr // nil = required
	}
	EventDecl struct {
		P    Pos
		Name string
	}
)

type ParamDef struct {
	Name string
	P    Pos
	T    *Type
}

func (d *StateDecl) Pos() Pos   { return d.P }
func (d *LetDecl) Pos() Pos     { return d.P }
func (d *FnDecl) Pos() Pos      { return d.P }
func (d *HandlerDecl) Pos() Pos { return d.P }
func (d *DataDecl) Pos() Pos    { return d.P }
func (d *ParamDecl) Pos() Pos   { return d.P }
func (d *EventDecl) Pos() Pos   { return d.P }

func (d *StateDecl) DeclName() string   { return d.Name }
func (d *LetDecl) DeclName() string     { return d.Name }
func (d *FnDecl) DeclName() string      { return d.Name }
func (d *HandlerDecl) DeclName() string { return d.Name }
func (d *DataDecl) DeclName() string    { return d.Name }
func (d *ParamDecl) DeclName() string   { return d.Name }
func (d *EventDecl) DeclName() string   { return d.Name }
