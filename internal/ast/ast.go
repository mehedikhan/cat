// Package ast defines the cat syntax tree.
package ast

// Pos is a position in the original .cat file.
type Pos struct {
	Line int
	Col  int
}

func (p Pos) Position() Pos { return p }

type Node interface{ Position() Pos }

type Stmt interface {
	Node
	stmt()
}

type Expr interface {
	Node
	expr()
}

// ---------- types ----------

// TypeNode is a type annotation: Int, String, Array(Int), Channel(Point), Point.
type TypeNode struct {
	Pos
	Name string
	Args []*TypeNode
}

func (t *TypeNode) String() string {
	if len(t.Args) == 0 {
		return t.Name
	}
	s := t.Name + "("
	for i, a := range t.Args {
		if i > 0 {
			s += ", "
		}
		s += a.String()
	}
	return s + ")"
}

// ---------- declarations ----------

type Param struct {
	Pos
	Name string
	Type *TypeNode
}

type Field struct {
	Pos
	Name string
	Type *TypeNode
}

type FuncDecl struct {
	Pos
	Name   string
	Recv   string // owning struct name, "" for a free function
	Params []*Param
	Ret    *TypeNode
	Body   []Stmt
}

type StructDecl struct {
	Pos
	Name    string
	Fields  []*Field
	Methods []*FuncDecl
}

// Program is a whole .cat file. Top-level statements become main().
type Program struct {
	Structs []*StructDecl
	Funcs   []*FuncDecl
	Main    []Stmt
}

// ---------- statements ----------

type ExprStmt struct {
	Pos
	X Expr
}

type AssignStmt struct {
	Pos
	Target Expr // Ident, FieldExpr or IndexExpr
	Value  Expr
}

// VarDecl is an explicitly typed binding: x : Int = 0
type VarDecl struct {
	Pos
	Name  string
	Type  *TypeNode
	Value Expr // may be nil
}

type Branch struct {
	Pos
	Cond Expr
	Body []Stmt
}

type IfStmt struct {
	Pos
	Cond  Expr
	Then  []Stmt
	Elifs []*Branch
	Else  []Stmt // nil when absent
}

type WhileStmt struct {
	Pos
	Cond Expr
	Body []Stmt
}

type ReturnStmt struct {
	Pos
	Value Expr // may be nil
}

type BreakStmt struct{ Pos }
type NextStmt struct{ Pos }

type SpawnStmt struct {
	Pos
	Body []Stmt
}

type DeferStmt struct {
	Pos
	Body []Stmt
}

func (*ExprStmt) stmt()   {}
func (*AssignStmt) stmt() {}
func (*VarDecl) stmt()    {}
func (*IfStmt) stmt()     {}
func (*WhileStmt) stmt()  {}
func (*ReturnStmt) stmt() {}
func (*BreakStmt) stmt()  {}
func (*NextStmt) stmt()   {}
func (*SpawnStmt) stmt()  {}
func (*DeferStmt) stmt()  {}

// ---------- expressions ----------

type IntLit struct {
	Pos
	Val string
}

type FloatLit struct {
	Pos
	Val string
}

// StrPart is literal text (Text) or an interpolated expression (X).
type StrPart struct {
	Text string
	X    Expr
}

type StrLit struct {
	Pos
	Parts []StrPart
}

type BoolLit struct {
	Pos
	Val bool
}

type NilLit struct{ Pos }

type Ident struct {
	Pos
	Name string
}

// IVar is @name, shorthand for self.name inside a method.
type IVar struct {
	Pos
	Name string
}

type SelfExpr struct{ Pos }

type ArrayLit struct {
	Pos
	Elems []Expr
}

type BinaryExpr struct {
	Pos
	Op string
	L  Expr
	R  Expr
}

type UnaryExpr struct {
	Pos
	Op string
	X  Expr
}

// Block is a do |a, b| ... end (or { |a, b| ... }) attached to a call.
type Block struct {
	Pos
	Params []string
	Body   []Stmt
}

type CallExpr struct {
	Pos
	Name  string
	Args  []Expr
	Block *Block
}

type KeyVal struct {
	Pos
	Name  string
	Value Expr
}

type MethodCall struct {
	Pos
	Recv   Expr
	Name   string
	Args   []Expr
	KwArgs []*KeyVal
	Block  *Block
	Parens bool
}

// FieldExpr is recv.name with no arguments and no parentheses.
type FieldExpr struct {
	Pos
	Recv Expr
	Name string
}

type IndexExpr struct {
	Pos
	Recv  Expr
	Index Expr
}

type RangeExpr struct {
	Pos
	Lo        Expr
	Hi        Expr
	Exclusive bool // ... excludes Hi, .. includes it
}

// TypeRef is a type used in expression position, as in Channel(Int).new(4).
type TypeRef struct {
	Pos
	Type *TypeNode
}

// NewExpr is T.new(...): a struct literal or a channel.
type NewExpr struct {
	Pos
	Type   *TypeNode
	Args   []Expr
	KwArgs []*KeyVal
}

func (*IntLit) expr()     {}
func (*FloatLit) expr()   {}
func (*StrLit) expr()     {}
func (*BoolLit) expr()    {}
func (*NilLit) expr()     {}
func (*Ident) expr()      {}
func (*IVar) expr()       {}
func (*SelfExpr) expr()   {}
func (*ArrayLit) expr()   {}
func (*BinaryExpr) expr() {}
func (*UnaryExpr) expr()  {}
func (*CallExpr) expr()   {}
func (*MethodCall) expr() {}
func (*FieldExpr) expr()  {}
func (*IndexExpr) expr()  {}
func (*RangeExpr) expr()  {}
func (*TypeRef) expr()    {}
func (*NewExpr) expr()    {}
