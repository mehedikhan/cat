package parser

import (
	"testing"

	"github.com/mehedikhan/cat/internal/ast"
)

func parse(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, err := Parse("test.cat", src)
	if err != nil {
		t.Fatalf("parse(%q): %v", src, err)
	}
	return prog
}

func TestNewlineTerminatesStatements(t *testing.T) {
	prog := parse(t, "x = 1\ny = 2\n")
	if len(prog.Main) != 2 {
		t.Fatalf("got %d statements, want 2", len(prog.Main))
	}
}

func TestNewlineContinuesAfterOperator(t *testing.T) {
	prog := parse(t, "x = 1 +\n2\n")
	if len(prog.Main) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Main))
	}
}

func TestPrecedence(t *testing.T) {
	prog := parse(t, "x = 1 + 2 * 3\n")
	bin := prog.Main[0].(*ast.AssignStmt).Value.(*ast.BinaryExpr)
	if bin.Op != "+" {
		t.Fatalf("top operator is %q, want +", bin.Op)
	}
	if bin.R.(*ast.BinaryExpr).Op != "*" {
		t.Fatal("multiplication should bind tighter than addition")
	}
}

func TestStringInterpolation(t *testing.T) {
	prog := parse(t, `puts "a#{1 + 2}b"`)
	lit := prog.Main[0].(*ast.ExprStmt).X.(*ast.CallExpr).Args[0].(*ast.StrLit)
	if len(lit.Parts) != 3 {
		t.Fatalf("got %d parts, want 3", len(lit.Parts))
	}
	if lit.Parts[1].X == nil {
		t.Fatal("middle part should be an expression")
	}
}

func TestStructWithMethod(t *testing.T) {
	prog := parse(t, "struct P\n  x : Int\n  def get : Int\n    @x\n  end\nend\n")
	if len(prog.Structs) != 1 {
		t.Fatalf("got %d structs, want 1", len(prog.Structs))
	}
	s := prog.Structs[0]
	if len(s.Fields) != 1 || len(s.Methods) != 1 {
		t.Fatalf("got %d fields and %d methods, want 1 and 1", len(s.Fields), len(s.Methods))
	}
	if s.Methods[0].Ret.Name != "Int" {
		t.Fatalf("return type is %q, want Int", s.Methods[0].Ret.Name)
	}
}

func TestBlockParams(t *testing.T) {
	prog := parse(t, "xs.each do |x, i|\n  puts x\nend\n")
	call := prog.Main[0].(*ast.ExprStmt).X.(*ast.MethodCall)
	if len(call.Block.Params) != 2 {
		t.Fatalf("got %d block params, want 2", len(call.Block.Params))
	}
}

func TestModifierIf(t *testing.T) {
	prog := parse(t, "puts 1 if x > 0\n")
	if _, ok := prog.Main[0].(*ast.IfStmt); !ok {
		t.Fatalf("got %T, want *ast.IfStmt", prog.Main[0])
	}
}

func TestParseErrorsHavePositions(t *testing.T) {
	_, err := Parse("test.cat", "x = \n")
	if err == nil {
		t.Fatal("expected an error")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("got %T, want *parser.Error", err)
	}
	if e.Line == 0 {
		t.Fatal("error has no line number")
	}
}
