package lexer

import "testing"

func types(t *testing.T, src string) []Type {
	t.Helper()
	toks, err := Tokenize("test.cat", src)
	if err != nil {
		t.Fatalf("tokenize(%q): %v", src, err)
	}
	out := make([]Type, len(toks))
	for i, tk := range toks {
		out[i] = tk.Type
	}
	return out
}

func TestNewlineSuppression(t *testing.T) {
	got := types(t, "1 +\n2\n")
	want := []Type{INT, PLUS, INT, NEWLINE, EOF}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestRangeIsNotAFloat(t *testing.T) {
	got := types(t, "1..5")
	if got[0] != INT || got[1] != DOTDOT || got[2] != INT {
		t.Fatalf("got %v, want INT DOTDOT INT", got)
	}
}

func TestQuestionMarkMethodName(t *testing.T) {
	toks, err := Tokenize("test.cat", "empty? x != y")
	if err != nil {
		t.Fatal(err)
	}
	if toks[0].Lit != "empty?" {
		t.Fatalf("got %q, want empty?", toks[0].Lit)
	}
	if toks[2].Type != NEQ {
		t.Fatalf("got %v, want !=", toks[2].Type)
	}
}

func TestInterpolationParts(t *testing.T) {
	toks, err := Tokenize("test.cat", `"a#{b}c"`)
	if err != nil {
		t.Fatal(err)
	}
	parts := toks[0].Parts
	if len(parts) != 3 || !parts[1].IsExpr || parts[1].Text != "b" {
		t.Fatalf("got %+v", parts)
	}
}

func TestUnterminatedString(t *testing.T) {
	if _, err := Tokenize("test.cat", `"oops`); err == nil {
		t.Fatal("expected an error")
	}
}
