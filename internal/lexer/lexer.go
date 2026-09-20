// Package lexer turns cat source text into a flat token slice.
//
// The Ruby-flavoured parts live here: newlines terminate statements (unless the
// previous token cannot end one), identifiers may end in ? or !, and strings
// carry their #{...} interpolation segments as unparsed source.
package lexer

import "fmt"

type Type int

const (
	EOF Type = iota
	NEWLINE
	IDENT
	CONST
	IVAR
	INT
	FLOAT
	STRING

	DEF
	END
	IF
	ELSIF
	ELSE
	UNLESS
	WHILE
	RETURN
	STRUCT
	SPAWN
	DEFER
	DO
	THEN
	NIL
	TRUE
	FALSE
	AND
	OR
	NOT
	BREAK
	NEXT
	SELF

	PLUS
	MINUS
	STAR
	SLASH
	PERCENT
	POW
	EQ
	NEQ
	LT
	LTE
	GT
	GTE
	ASSIGN
	SHL
	ANDAND
	OROR
	BANG
	DOT
	DOTDOT
	DOTDOTDOT
	COMMA
	COLON
	LPAREN
	RPAREN
	LBRACKET
	RBRACKET
	LBRACE
	RBRACE
	PIPE
)

var names = map[Type]string{
	EOF: "end of file", NEWLINE: "newline", IDENT: "identifier", CONST: "constant",
	IVAR: "instance variable", INT: "int", FLOAT: "float", STRING: "string",
	DEF: "def", END: "end", IF: "if", ELSIF: "elsif", ELSE: "else", UNLESS: "unless",
	WHILE: "while", RETURN: "return", STRUCT: "struct", SPAWN: "spawn", DEFER: "defer",
	DO: "do", THEN: "then", NIL: "nil", TRUE: "true", FALSE: "false", AND: "and",
	OR: "or", NOT: "not", BREAK: "break", NEXT: "next", SELF: "self",
	PLUS: "+", MINUS: "-", STAR: "*", SLASH: "/", PERCENT: "%", POW: "**",
	EQ: "==", NEQ: "!=", LT: "<", LTE: "<=", GT: ">", GTE: ">=", ASSIGN: "=",
	SHL: "<<", ANDAND: "&&", OROR: "||", BANG: "!", DOT: ".", DOTDOT: "..",
	DOTDOTDOT: "...", COMMA: ",", COLON: ":", LPAREN: "(", RPAREN: ")",
	LBRACKET: "[", RBRACKET: "]", LBRACE: "{", RBRACE: "}", PIPE: "|",
}

func (t Type) String() string {
	if n, ok := names[t]; ok {
		return n
	}
	return fmt.Sprintf("token(%d)", int(t))
}

var keywords = map[string]Type{
	"def": DEF, "end": END, "if": IF, "elsif": ELSIF, "else": ELSE,
	"unless": UNLESS, "while": WHILE, "return": RETURN, "struct": STRUCT,
	"spawn": SPAWN, "defer": DEFER, "do": DO, "then": THEN, "nil": NIL,
	"true": TRUE, "false": FALSE, "and": AND, "or": OR, "not": NOT,
	"break": BREAK, "next": NEXT, "self": SELF,
}

// A Part is one segment of a string literal: either literal text or the raw
// source of an interpolated expression, which the parser re-parses.
type Part struct {
	IsExpr bool
	Text   string
	Line   int
	Col    int
}

type Token struct {
	Type  Type
	Lit   string
	Parts []Part
	Line  int
	Col   int
}

func (t Token) String() string {
	if t.Lit != "" {
		return fmt.Sprintf("%s(%s)", t.Type, t.Lit)
	}
	return t.Type.String()
}

// Error is a lexical error carrying a cat source position.
type Error struct {
	File string
	Line int
	Col  int
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg) }

// noTerminate lists tokens after which a newline is a line continuation rather
// than a statement terminator.
var noTerminate = map[Type]bool{
	PLUS: true, MINUS: true, STAR: true, SLASH: true, PERCENT: true, POW: true,
	EQ: true, NEQ: true, LT: true, LTE: true, GT: true, GTE: true, ASSIGN: true,
	SHL: true, ANDAND: true, OROR: true, BANG: true, DOT: true, DOTDOT: true,
	DOTDOTDOT: true, COMMA: true, COLON: true, LPAREN: true, LBRACKET: true,
	LBRACE: true, PIPE: true, AND: true, OR: true, NOT: true, DO: true,
	THEN: true, ELSE: true, NEWLINE: true,
}

type lexer struct {
	file string
	src  string
	pos  int
	line int
	col  int
	toks []Token
}

// Tokenize lexes src, reporting positions against file.
func Tokenize(file, src string) ([]Token, error) {
	l := &lexer{file: file, src: src, line: 1, col: 1}
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		if t.Type == NEWLINE {
			if len(l.toks) == 0 || noTerminate[l.toks[len(l.toks)-1].Type] {
				continue
			}
		}
		l.toks = append(l.toks, t)
		if t.Type == EOF {
			return l.toks, nil
		}
	}
}

func (l *lexer) errf(line, col int, format string, args ...any) error {
	return &Error{File: l.file, Line: line, Col: col, Msg: fmt.Sprintf(format, args...)}
}

func (l *lexer) at(off int) byte {
	if l.pos+off >= len(l.src) {
		return 0
	}
	return l.src[l.pos+off]
}

func (l *lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

func (l *lexer) tok(t Type, lit string, line, col int) Token {
	return Token{Type: t, Lit: lit, Line: line, Col: col}
}

func (l *lexer) next() (Token, error) {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			l.advance()
		case c == '\\' && l.at(1) == '\n':
			l.advance()
			l.advance()
		case c == '#':
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.advance()
			}
		default:
			goto scan
		}
	}
scan:
	if l.pos >= len(l.src) {
		return l.tok(EOF, "", l.line, l.col), nil
	}
	line, col := l.line, l.col
	c := l.src[l.pos]

	switch {
	case c == '\n':
		l.advance()
		return l.tok(NEWLINE, "", line, col), nil
	case isDigit(c):
		return l.lexNumber()
	case isAlpha(c):
		return l.lexIdent()
	case c == '@':
		l.advance()
		if !isAlpha(l.at(0)) {
			return Token{}, l.errf(line, col, "expected a name after @")
		}
		start := l.pos
		for isAlnum(l.at(0)) {
			l.advance()
		}
		return l.tok(IVAR, l.src[start:l.pos], line, col), nil
	case c == '"':
		return l.lexString()
	}

	two := ""
	if l.pos+1 < len(l.src) {
		two = l.src[l.pos : l.pos+2]
	}
	three := ""
	if l.pos+2 < len(l.src) {
		three = l.src[l.pos : l.pos+3]
	}

	if three == "..." {
		l.advance()
		l.advance()
		l.advance()
		return l.tok(DOTDOTDOT, "", line, col), nil
	}
	if t, ok := map[string]Type{
		"**": POW, "==": EQ, "!=": NEQ, "<=": LTE, ">=": GTE,
		"<<": SHL, "&&": ANDAND, "||": OROR, "..": DOTDOT,
	}[two]; ok {
		l.advance()
		l.advance()
		return l.tok(t, "", line, col), nil
	}
	if t, ok := map[byte]Type{
		'+': PLUS, '-': MINUS, '*': STAR, '/': SLASH, '%': PERCENT,
		'<': LT, '>': GT, '=': ASSIGN, '!': BANG, '.': DOT, ',': COMMA,
		':': COLON, '(': LPAREN, ')': RPAREN, '[': LBRACKET, ']': RBRACKET,
		'{': LBRACE, '}': RBRACE, '|': PIPE,
	}[c]; ok {
		l.advance()
		return l.tok(t, "", line, col), nil
	}
	return Token{}, l.errf(line, col, "unexpected character %q", string(c))
}

func (l *lexer) lexNumber() (Token, error) {
	line, col := l.line, l.col
	var digits []byte
	for isDigit(l.at(0)) || l.at(0) == '_' {
		c := l.advance()
		if c != '_' {
			digits = append(digits, c)
		}
	}
	// "1..5" is a range, not a float.
	if l.at(0) == '.' && isDigit(l.at(1)) {
		digits = append(digits, l.advance())
		for isDigit(l.at(0)) || l.at(0) == '_' {
			c := l.advance()
			if c != '_' {
				digits = append(digits, c)
			}
		}
		return l.tok(FLOAT, string(digits), line, col), nil
	}
	return l.tok(INT, string(digits), line, col), nil
}

func (l *lexer) lexIdent() (Token, error) {
	line, col := l.line, l.col
	start := l.pos
	for isAlnum(l.at(0)) {
		l.advance()
	}
	// Trailing ? or ! is part of the name (empty?, save!), but "x != y" is not.
	if l.at(0) == '?' || (l.at(0) == '!' && l.at(1) != '=') {
		l.advance()
	}
	word := l.src[start:l.pos]
	if t, ok := keywords[word]; ok {
		return l.tok(t, word, line, col), nil
	}
	if word[0] >= 'A' && word[0] <= 'Z' {
		return l.tok(CONST, word, line, col), nil
	}
	return l.tok(IDENT, word, line, col), nil
}

func (l *lexer) lexString() (Token, error) {
	line, col := l.line, l.col
	l.advance() // opening quote
	var parts []Part
	var text []byte
	flush := func() {
		if len(text) > 0 {
			parts = append(parts, Part{Text: string(text)})
			text = nil
		}
	}
	for {
		if l.pos >= len(l.src) {
			return Token{}, l.errf(line, col, "unterminated string")
		}
		c := l.src[l.pos]
		if c == '"' {
			l.advance()
			break
		}
		if c == '\\' {
			l.advance()
			if l.pos >= len(l.src) {
				return Token{}, l.errf(line, col, "unterminated string")
			}
			e := l.advance()
			switch e {
			case 'n':
				text = append(text, '\n')
			case 't':
				text = append(text, '\t')
			case 'r':
				text = append(text, '\r')
			default:
				text = append(text, e)
			}
			continue
		}
		if c == '#' && l.at(1) == '{' {
			flush()
			l.advance()
			l.advance()
			eLine, eCol := l.line, l.col
			start := l.pos
			depth := 1
			for {
				if l.pos >= len(l.src) {
					return Token{}, l.errf(eLine, eCol, "unterminated #{} interpolation")
				}
				switch l.src[l.pos] {
				case '{':
					depth++
				case '}':
					depth--
				}
				if depth == 0 {
					break
				}
				l.advance()
			}
			parts = append(parts, Part{IsExpr: true, Text: l.src[start:l.pos], Line: eLine, Col: eCol})
			l.advance() // closing }
			continue
		}
		text = append(text, l.advance())
	}
	flush()
	if len(parts) == 0 {
		parts = append(parts, Part{Text: ""})
	}
	return Token{Type: STRING, Parts: parts, Line: line, Col: col}, nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isAlnum(c byte) bool { return isAlpha(c) || isDigit(c) }
