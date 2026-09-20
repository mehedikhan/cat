// Package codegen lowers a cat program to Go source.
//
// Output is written as text with hand-managed indentation rather than through
// go/printer, so that every emitted line can be recorded in a line map. The
// driver uses that map to rewrite Go compiler errors back onto .cat positions.
package codegen

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mehedikhan/cat/internal/ast"
)

type Error struct {
	File string
	Line int
	Col  int
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg) }

// Result is generated Go source plus a map from Go line to cat line.
type Result struct {
	Source  string
	LineMap map[int]int
}

// variable kinds we track well enough to lower << , .each and friends
const (
	kindUnknown = ""
	kindChan    = "chan"
	kindSlice   = "slice"
	kindRange   = "range"
)

type gen struct {
	file    string
	buf     strings.Builder
	indent  int
	line    int
	lineMap map[int]int
	imports map[string]bool
	structs map[string]*ast.StructDecl
	methods map[string]bool
	fields  map[string]bool
	scopes  []map[string]string
	errs    []error
	curFn   *ast.FuncDecl
	tmp     int
}

// Generate lowers prog to Go source.
func Generate(file string, prog *ast.Program) (*Result, error) {
	g := &gen{
		file:    file,
		lineMap: map[int]int{},
		imports: map[string]bool{},
		structs: map[string]*ast.StructDecl{},
		methods: map[string]bool{},
		fields:  map[string]bool{},
	}
	for _, s := range prog.Structs {
		g.structs[s.Name] = s
		for _, f := range s.Fields {
			g.fields[f.Name] = true
		}
		for _, m := range s.Methods {
			g.methods[m.Name] = true
		}
	}
	g.program(prog)
	if len(g.errs) > 0 {
		return nil, g.errs[0]
	}

	var head strings.Builder
	head.WriteString("package main\n\n")
	if len(g.imports) > 0 {
		names := make([]string, 0, len(g.imports))
		for n := range g.imports {
			names = append(names, n)
		}
		sort.Strings(names)
		head.WriteString("import (\n")
		for _, n := range names {
			head.WriteString("\t" + strconv.Quote(n) + "\n")
		}
		head.WriteString(")\n\n")
	}
	offset := strings.Count(head.String(), "\n")
	shifted := make(map[int]int, len(g.lineMap))
	for goLine, catLine := range g.lineMap {
		shifted[goLine+offset] = catLine
	}
	return &Result{Source: head.String() + g.buf.String(), LineMap: shifted}, nil
}

// ---------- emit helpers ----------

func (g *gen) emit(pos ast.Pos, s string) {
	g.buf.WriteString(strings.Repeat("\t", g.indent))
	g.buf.WriteString(s)
	g.buf.WriteByte('\n')
	g.line++
	if pos.Line > 0 {
		g.lineMap[g.line] = pos.Line
	}
}

func (g *gen) blank() { g.emit(ast.Pos{}, "") }

func (g *gen) errf(pos ast.Pos, format string, args ...any) string {
	g.errs = append(g.errs, &Error{File: g.file, Line: pos.Line, Col: pos.Col, Msg: fmt.Sprintf(format, args...)})
	return "nil /* error */"
}

func (g *gen) need(pkg string) { g.imports[pkg] = true }

func (g *gen) push() { g.scopes = append(g.scopes, map[string]string{}) }
func (g *gen) pop()  { g.scopes = g.scopes[:len(g.scopes)-1] }

func (g *gen) declare(name, kind string) {
	if len(g.scopes) == 0 {
		g.push()
	}
	g.scopes[len(g.scopes)-1][name] = kind
}

func (g *gen) declared(name string) bool {
	for i := len(g.scopes) - 1; i >= 0; i-- {
		if _, ok := g.scopes[i][name]; ok {
			return true
		}
	}
	return false
}

func (g *gen) kindOfVar(name string) string {
	for i := len(g.scopes) - 1; i >= 0; i-- {
		if k, ok := g.scopes[i][name]; ok {
			return k
		}
	}
	return kindUnknown
}

func (g *gen) temp(prefix string) string {
	g.tmp++
	return fmt.Sprintf("_%s%d", prefix, g.tmp)
}

// ---------- names and types ----------

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
	"len": true, "cap": true, "make": true, "new": true, "copy": true,
	"append": true, "string": true, "int": true, "float64": true, "bool": true,
	"byte": true, "rune": true, "error": true, "nil": true, "true": true, "false": true,
}

// mangle turns a cat name into a legal, non-colliding Go identifier.
func mangle(name string) string {
	switch {
	case strings.HasSuffix(name, "?"):
		name = strings.TrimSuffix(name, "?") + "_p"
	case strings.HasSuffix(name, "!"):
		name = strings.TrimSuffix(name, "!") + "_bang"
	}
	if goKeywords[name] {
		return name + "_"
	}
	return name
}

var primitives = map[string]string{
	"Int": "int", "Int64": "int64", "Float": "float64", "String": "string",
	"Bool": "bool", "Byte": "byte", "Bytes": "[]byte",
}

func (g *gen) goType(t *ast.TypeNode) string {
	if t == nil {
		return ""
	}
	if p, ok := primitives[t.Name]; ok && len(t.Args) == 0 {
		return p
	}
	switch t.Name {
	case "Array":
		if len(t.Args) != 1 {
			return g.errf(t.Pos, "Array takes one type argument, got %d", len(t.Args))
		}
		return "[]" + g.goType(t.Args[0])
	case "Channel":
		if len(t.Args) != 1 {
			return g.errf(t.Pos, "Channel takes one type argument, got %d", len(t.Args))
		}
		return "chan " + g.goType(t.Args[0])
	}
	if len(t.Args) > 0 {
		return g.errf(t.Pos, "unknown generic type %s", t.Name)
	}
	if _, ok := g.structs[t.Name]; ok {
		return t.Name
	}
	return g.errf(t.Pos, "unknown type %s", t.Name)
}

func kindOfType(t *ast.TypeNode) string {
	if t == nil {
		return kindUnknown
	}
	switch t.Name {
	case "Array":
		return kindSlice
	case "Channel":
		return kindChan
	}
	return kindUnknown
}

// kindOf is a best-effort guess at what an expression evaluates to.
func (g *gen) kindOf(x ast.Expr) string {
	switch v := x.(type) {
	case *ast.Ident:
		return g.kindOfVar(v.Name)
	case *ast.ArrayLit:
		return kindSlice
	case *ast.RangeExpr:
		return kindRange
	case *ast.NewExpr:
		return kindOfType(v.Type)
	case *ast.TypeRef:
		return kindOfType(v.Type)
	}
	return kindUnknown
}

// ---------- program ----------

func (g *gen) program(prog *ast.Program) {
	g.push()
	defer g.pop()

	for _, s := range prog.Structs {
		g.structDecl(s)
	}
	for _, f := range prog.Funcs {
		g.funcDecl(f)
	}
	for _, s := range prog.Structs {
		for _, m := range s.Methods {
			g.funcDecl(m)
		}
	}

	g.emit(ast.Pos{}, "func main() {")
	g.indent++
	g.push()
	g.stmts(prog.Main)
	g.pop()
	g.indent--
	g.emit(ast.Pos{}, "}")
}

func (g *gen) structDecl(s *ast.StructDecl) {
	g.emit(s.Pos, "type "+s.Name+" struct {")
	g.indent++
	for _, f := range s.Fields {
		g.emit(f.Pos, mangle(f.Name)+" "+g.goType(f.Type))
	}
	g.indent--
	g.emit(ast.Pos{}, "}")
	g.blank()
}

func (g *gen) funcDecl(f *ast.FuncDecl) {
	var sb strings.Builder
	sb.WriteString("func ")
	if f.Recv != "" {
		sb.WriteString("(self *" + f.Recv + ") ")
	}
	sb.WriteString(mangle(f.Name) + "(")
	for i, p := range f.Params {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(mangle(p.Name) + " " + g.goType(p.Type))
	}
	sb.WriteString(")")
	if f.Ret != nil {
		sb.WriteString(" " + g.goType(f.Ret))
	}
	sb.WriteString(" {")
	g.emit(f.Pos, sb.String())

	g.indent++
	g.push()
	for _, p := range f.Params {
		g.declare(p.Name, kindOfType(p.Type))
	}
	prev := g.curFn
	g.curFn = f
	g.funcBody(f)
	g.curFn = prev
	g.pop()
	g.indent--
	g.emit(ast.Pos{}, "}")
	g.blank()
}

// funcBody emits the body, turning a trailing expression into a return when
// the function declares a result type.
func (g *gen) funcBody(f *ast.FuncDecl) {
	body := f.Body
	returns := false
	if f.Ret != nil && len(body) > 0 {
		if es, ok := body[len(body)-1].(*ast.ExprStmt); ok {
			g.stmts(body[:len(body)-1])
			g.emit(es.Pos, "return "+g.expr(es.X))
			return
		}
		if _, ok := body[len(body)-1].(*ast.ReturnStmt); ok {
			returns = true
		}
	}
	g.stmts(body)
	if f.Ret != nil && !returns {
		// Go demands a terminating statement; cat lets branches do the returning.
		g.emit(ast.Pos{}, "panic("+strconv.Quote("cat: "+f.Name+" ended without returning a value")+")")
	}
}

// ---------- statements ----------

func (g *gen) stmts(list []ast.Stmt) {
	for _, s := range list {
		g.stmt(s)
	}
}

func (g *gen) stmt(s ast.Stmt) {
	switch v := s.(type) {
	case *ast.ExprStmt:
		g.exprStmt(v)
	case *ast.AssignStmt:
		g.assign(v)
	case *ast.VarDecl:
		g.varDecl(v)
	case *ast.IfStmt:
		g.ifStmt(v)
	case *ast.WhileStmt:
		g.emit(v.Pos, "for "+g.expr(v.Cond)+" {")
		g.indent++
		g.push()
		g.stmts(v.Body)
		g.pop()
		g.indent--
		g.emit(ast.Pos{}, "}")
	case *ast.ReturnStmt:
		if v.Value == nil {
			g.emit(v.Pos, "return")
		} else {
			g.emit(v.Pos, "return "+g.expr(v.Value))
		}
	case *ast.BreakStmt:
		g.emit(v.Pos, "break")
	case *ast.NextStmt:
		g.emit(v.Pos, "continue")
	case *ast.SpawnStmt:
		g.emit(v.Pos, "go func() {")
		g.indent++
		g.push()
		g.stmts(v.Body)
		g.pop()
		g.indent--
		g.emit(ast.Pos{}, "}()")
	case *ast.DeferStmt:
		g.emit(v.Pos, "defer func() {")
		g.indent++
		g.push()
		g.stmts(v.Body)
		g.pop()
		g.indent--
		g.emit(ast.Pos{}, "}()")
	default:
		g.errf(s.Position(), "unsupported statement %T", s)
	}
}

func (g *gen) ifStmt(v *ast.IfStmt) {
	g.emit(v.Pos, "if "+g.expr(v.Cond)+" {")
	g.indent++
	g.push()
	g.stmts(v.Then)
	g.pop()
	g.indent--
	for _, b := range v.Elifs {
		g.emit(b.Pos, "} else if "+g.expr(b.Cond)+" {")
		g.indent++
		g.push()
		g.stmts(b.Body)
		g.pop()
		g.indent--
	}
	if v.Else != nil {
		g.emit(ast.Pos{}, "} else {")
		g.indent++
		g.push()
		g.stmts(v.Else)
		g.pop()
		g.indent--
	}
	g.emit(ast.Pos{}, "}")
}

func (g *gen) varDecl(v *ast.VarDecl) {
	typ := g.goType(v.Type)
	name := mangle(v.Name)
	if v.Value == nil {
		g.emit(v.Pos, "var "+name+" "+typ)
	} else {
		g.emit(v.Pos, "var "+name+" "+typ+" = "+g.exprHint(v.Value, v.Type))
	}
	g.declare(v.Name, kindOfType(v.Type))
	g.emit(ast.Pos{}, "_ = "+name)
}

func (g *gen) assign(v *ast.AssignStmt) {
	if id, ok := v.Target.(*ast.Ident); ok {
		name := mangle(id.Name)
		if g.declared(id.Name) {
			g.emit(v.Pos, name+" = "+g.expr(v.Value))
			return
		}
		g.emit(v.Pos, name+" := "+g.expr(v.Value))
		g.declare(id.Name, g.kindOf(v.Value))
		g.emit(ast.Pos{}, "_ = "+name)
		return
	}
	g.emit(v.Pos, g.expr(v.Target)+" = "+g.expr(v.Value))
}

// exprStmt handles the forms that only make sense as statements: channel
// sends, array pushes, and block-taking iterators lowered to for loops.
func (g *gen) exprStmt(s *ast.ExprStmt) {
	switch v := s.X.(type) {
	case *ast.BinaryExpr:
		if v.Op == "<<" {
			g.shiftStmt(v)
			return
		}
	case *ast.MethodCall:
		if v.Block != nil {
			g.loop(v)
			return
		}
		if v.Name == "push" && len(v.Args) == 1 {
			target := g.expr(v.Recv)
			g.emit(s.Pos, target+" = append("+target+", "+g.expr(v.Args[0])+")")
			return
		}
	case *ast.FieldExpr:
		if v.Name == "close" {
			g.emit(s.Pos, "close("+g.expr(v.Recv)+")")
			return
		}
	}
	g.emit(s.Pos, g.expr(s.X))
}

// shiftStmt lowers `x << v` to a channel send or an append.
func (g *gen) shiftStmt(v *ast.BinaryExpr) {
	switch g.kindOf(v.L) {
	case kindChan:
		g.emit(v.Pos, g.expr(v.L)+" <- "+g.expr(v.R))
	case kindSlice:
		target := g.expr(v.L)
		g.emit(v.Pos, target+" = append("+target+", "+g.expr(v.R)+")")
	default:
		g.errf(v.Pos, "cannot tell whether << is a channel send or an array push; give the variable a type, e.g. `xs : Array(Int) = []`")
	}
}

// loop lowers each / each_with_index / times with a block into a for loop, so
// that break and next behave the way a Ruby programmer expects.
func (g *gen) loop(v *ast.MethodCall) {
	blk := v.Block
	body := func(decls []string) {
		g.indent++
		g.push()
		for _, d := range decls {
			g.declare(d, kindUnknown)
			g.emit(ast.Pos{}, "_ = "+mangle(d))
		}
		g.stmts(blk.Body)
		g.pop()
		g.indent--
		g.emit(ast.Pos{}, "}")
	}

	switch v.Name {
	case "times":
		idx := g.temp("i")
		if len(blk.Params) > 0 {
			idx = mangle(blk.Params[0])
		}
		n := g.expr(v.Recv)
		g.emit(v.Pos, "for "+idx+" := 0; "+idx+" < "+n+"; "+idx+"++ {")
		body(blk.Params)
	case "each":
		if r, ok := v.Recv.(*ast.RangeExpr); ok {
			idx := g.temp("i")
			if len(blk.Params) > 0 {
				idx = mangle(blk.Params[0])
			}
			cmp := "<="
			if r.Exclusive {
				cmp = "<"
			}
			g.emit(v.Pos, "for "+idx+" := "+g.expr(r.Lo)+"; "+idx+" "+cmp+" "+g.expr(r.Hi)+"; "+idx+"++ {")
			body(blk.Params)
			return
		}
		name := g.temp("v")
		if len(blk.Params) > 0 {
			name = mangle(blk.Params[0])
		}
		if g.kindOf(v.Recv) == kindChan {
			g.emit(v.Pos, "for "+name+" := range "+g.expr(v.Recv)+" {")
		} else {
			g.emit(v.Pos, "for _, "+name+" := range "+g.expr(v.Recv)+" {")
		}
		body(blk.Params)
	case "each_with_index":
		val, idx := g.temp("v"), g.temp("i")
		if len(blk.Params) > 0 {
			val = mangle(blk.Params[0])
		}
		if len(blk.Params) > 1 {
			idx = mangle(blk.Params[1])
		}
		g.emit(v.Pos, "for "+idx+", "+val+" := range "+g.expr(v.Recv)+" {")
		body(blk.Params)
	default:
		g.errf(v.Pos, "%s does not take a block (v0.1 supports each, each_with_index and times)", v.Name)
	}
}

// ---------- expressions ----------

func (g *gen) expr(x ast.Expr) string { return g.exprHint(x, nil) }

func (g *gen) exprHint(x ast.Expr, hint *ast.TypeNode) string {
	switch v := x.(type) {
	case *ast.IntLit:
		return v.Val
	case *ast.FloatLit:
		return v.Val
	case *ast.BoolLit:
		return strconv.FormatBool(v.Val)
	case *ast.NilLit:
		return "nil"
	case *ast.StrLit:
		return g.strLit(v)
	case *ast.Ident:
		return mangle(v.Name)
	case *ast.IVar:
		return "self." + mangle(v.Name)
	case *ast.SelfExpr:
		return "self"
	case *ast.ArrayLit:
		return g.arrayLit(v, hint)
	case *ast.UnaryExpr:
		return "(" + v.Op + g.expr(v.X) + ")"
	case *ast.BinaryExpr:
		return g.binary(v)
	case *ast.IndexExpr:
		return g.expr(v.Recv) + "[" + g.expr(v.Index) + "]"
	case *ast.FieldExpr:
		return g.field(v)
	case *ast.CallExpr:
		return g.call(v)
	case *ast.MethodCall:
		return g.methodCall(v)
	case *ast.NewExpr:
		return g.newExpr(v)
	case *ast.RangeExpr:
		return g.errf(v.Pos, "a range is only valid as the receiver of .each")
	case *ast.TypeRef:
		return g.errf(v.Pos, "a type is only valid as the receiver of .new")
	}
	return g.errf(x.Position(), "unsupported expression %T", x)
}

func (g *gen) binary(v *ast.BinaryExpr) string {
	if v.Op == "**" {
		g.need("math")
		return "math.Pow(float64(" + g.expr(v.L) + "), float64(" + g.expr(v.R) + "))"
	}
	if v.Op == "<<" {
		return g.errf(v.Pos, "<< is only valid as a statement")
	}
	return "(" + g.expr(v.L) + " " + v.Op + " " + g.expr(v.R) + ")"
}

func (g *gen) strLit(v *ast.StrLit) string {
	if len(v.Parts) == 1 && v.Parts[0].X == nil {
		return strconv.Quote(v.Parts[0].Text)
	}
	var format strings.Builder
	var args []string
	for _, p := range v.Parts {
		if p.X == nil {
			format.WriteString(strings.ReplaceAll(p.Text, "%", "%%"))
			continue
		}
		format.WriteString("%v")
		args = append(args, g.expr(p.X))
	}
	g.need("fmt")
	return "fmt.Sprintf(" + strconv.Quote(format.String()) + ", " + strings.Join(args, ", ") + ")"
}

func (g *gen) arrayLit(v *ast.ArrayLit, hint *ast.TypeNode) string {
	elem := ""
	if hint != nil && hint.Name == "Array" && len(hint.Args) == 1 {
		elem = g.goType(hint.Args[0])
	} else if len(v.Elems) > 0 {
		elem = g.inferElem(v.Elems[0])
	}
	if elem == "" {
		return g.errf(v.Pos, "cannot infer the element type of this array; annotate it, e.g. `xs : Array(Int) = []`")
	}
	parts := make([]string, len(v.Elems))
	for i, e := range v.Elems {
		parts[i] = g.expr(e)
	}
	return "[]" + elem + "{" + strings.Join(parts, ", ") + "}"
}

func (g *gen) inferElem(x ast.Expr) string {
	switch v := x.(type) {
	case *ast.IntLit:
		return "int"
	case *ast.FloatLit:
		return "float64"
	case *ast.StrLit:
		return "string"
	case *ast.BoolLit:
		return "bool"
	case *ast.NewExpr:
		if _, ok := g.structs[v.Type.Name]; ok {
			return v.Type.Name
		}
	}
	return ""
}

var mathFuncs = map[string]string{
	"sqrt": "Sqrt", "abs": "Abs", "floor": "Floor", "ceil": "Ceil",
	"pow": "Pow", "max": "Max", "min": "Min", "round": "Round",
	"sin": "Sin", "cos": "Cos", "tan": "Tan", "log": "Log",
}

var stringFuncs = map[string]string{
	"upcase": "ToUpper", "downcase": "ToLower", "strip": "TrimSpace",
}

func (g *gen) field(v *ast.FieldExpr) string {
	switch v.Name {
	case "size", "length":
		return "len(" + g.expr(v.Recv) + ")"
	case "receive":
		return "(<-" + g.expr(v.Recv) + ")"
	}
	if id, ok := v.Recv.(*ast.Ident); ok && id.Name == "Math" {
		if v.Name == "pi" {
			g.need("math")
			return "math.Pi"
		}
	}
	if fn, ok := stringFuncs[v.Name]; ok {
		g.need("strings")
		return "strings." + fn + "(" + g.expr(v.Recv) + ")"
	}
	if v.Name == "to_s" {
		g.need("fmt")
		return "fmt.Sprint(" + g.expr(v.Recv) + ")"
	}
	// A bare recv.name is a field unless only a method by that name exists,
	// in which case it is a Ruby-style parenthesis-free call.
	if g.methods[v.Name] && !g.fields[v.Name] {
		return g.expr(v.Recv) + "." + mangle(v.Name) + "()"
	}
	return g.expr(v.Recv) + "." + mangle(v.Name)
}

func (g *gen) call(v *ast.CallExpr) string {
	args := make([]string, len(v.Args))
	for i, a := range v.Args {
		args[i] = g.expr(a)
	}
	switch v.Name {
	case "puts":
		g.need("fmt")
		return "fmt.Println(" + strings.Join(args, ", ") + ")"
	case "print":
		g.need("fmt")
		return "fmt.Print(" + strings.Join(args, ", ") + ")"
	case "p":
		g.need("fmt")
		if len(args) != 1 {
			return g.errf(v.Pos, "p takes exactly one argument")
		}
		return "fmt.Printf(\"%#v\\n\", " + args[0] + ")"
	}
	return mangle(v.Name) + "(" + strings.Join(args, ", ") + ")"
}

func (g *gen) methodCall(v *ast.MethodCall) string {
	args := make([]string, len(v.Args))
	for i, a := range v.Args {
		args[i] = g.expr(a)
	}
	if id, ok := v.Recv.(*ast.Ident); ok && id.Name == "Math" {
		if fn, ok := mathFuncs[v.Name]; ok {
			g.need("math")
			for i := range args {
				args[i] = "float64(" + args[i] + ")"
			}
			return "math." + fn + "(" + strings.Join(args, ", ") + ")"
		}
		return g.errf(v.Pos, "Math has no function %s", v.Name)
	}
	switch v.Name {
	case "size", "length":
		return "len(" + g.expr(v.Recv) + ")"
	case "receive":
		return "(<-" + g.expr(v.Recv) + ")"
	case "to_s":
		g.need("fmt")
		return "fmt.Sprint(" + g.expr(v.Recv) + ")"
	case "include?":
		if len(args) == 1 {
			g.need("strings")
			return "strings.Contains(" + g.expr(v.Recv) + ", " + args[0] + ")"
		}
	case "split":
		if len(args) == 1 {
			g.need("strings")
			return "strings.Split(" + g.expr(v.Recv) + ", " + args[0] + ")"
		}
	case "join":
		if len(args) == 1 {
			g.need("strings")
			return "strings.Join(" + g.expr(v.Recv) + ", " + args[0] + ")"
		}
	}
	if fn, ok := stringFuncs[v.Name]; ok && len(args) == 0 {
		g.need("strings")
		return "strings." + fn + "(" + g.expr(v.Recv) + ")"
	}
	return g.expr(v.Recv) + "." + mangle(v.Name) + "(" + strings.Join(args, ", ") + ")"
}

func (g *gen) newExpr(v *ast.NewExpr) string {
	switch v.Type.Name {
	case "Channel":
		typ := g.goType(v.Type)
		if len(v.Args) == 0 {
			return "make(" + typ + ")"
		}
		if len(v.Args) == 1 {
			return "make(" + typ + ", " + g.expr(v.Args[0]) + ")"
		}
		return g.errf(v.Pos, "Channel.new takes at most one argument (the buffer size)")
	case "Array":
		return g.goType(v.Type) + "{}"
	}
	s, ok := g.structs[v.Type.Name]
	if !ok {
		return g.errf(v.Pos, "unknown type %s", v.Type.Name)
	}
	if len(v.KwArgs) > 0 {
		if len(v.Args) > 0 {
			return g.errf(v.Pos, "mix of positional and named arguments in %s.new", s.Name)
		}
		parts := make([]string, len(v.KwArgs))
		for i, kv := range v.KwArgs {
			parts[i] = mangle(kv.Name) + ": " + g.expr(kv.Value)
		}
		return s.Name + "{" + strings.Join(parts, ", ") + "}"
	}
	if len(v.Args) != 0 && len(v.Args) != len(s.Fields) {
		return g.errf(v.Pos, "%s.new takes %d arguments, got %d", s.Name, len(s.Fields), len(v.Args))
	}
	parts := make([]string, len(v.Args))
	for i, a := range v.Args {
		parts[i] = g.exprHint(a, s.Fields[i].Type)
	}
	return s.Name + "{" + strings.Join(parts, ", ") + "}"
}
