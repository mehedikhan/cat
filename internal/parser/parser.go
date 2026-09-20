// Package parser builds a cat AST from a token stream.
//
// Statements are recursive descent; expressions use precedence climbing.
package parser

import (
	"fmt"

	"github.com/mehedikhan/cat/internal/ast"
	"github.com/mehedikhan/cat/internal/lexer"
)

type Error struct {
	File string
	Line int
	Col  int
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg) }

type parser struct {
	file string
	toks []lexer.Token
	pos  int
}

// Parse parses a whole source file.
func Parse(file, src string) (*ast.Program, error) {
	toks, err := lexer.Tokenize(file, src)
	if err != nil {
		return nil, err
	}
	p := &parser{file: file, toks: toks}
	return p.program()
}

// ---------- token helpers ----------

func (p *parser) cur() lexer.Token  { return p.toks[p.pos] }
func (p *parser) peek() lexer.Token { return p.toks[min(p.pos+1, len(p.toks)-1)] }

func (p *parser) at(t lexer.Type) bool { return p.cur().Type == t }

func (p *parser) advance() lexer.Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) accept(t lexer.Type) bool {
	if p.at(t) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) expect(t lexer.Type) (lexer.Token, error) {
	if !p.at(t) {
		return lexer.Token{}, p.errf("expected %s, found %s", t, p.cur())
	}
	return p.advance(), nil
}

func (p *parser) errf(format string, args ...any) error {
	t := p.cur()
	return &Error{File: p.file, Line: t.Line, Col: t.Col, Msg: fmt.Sprintf(format, args...)}
}

func (p *parser) pos_() ast.Pos {
	t := p.cur()
	return ast.Pos{Line: t.Line, Col: t.Col}
}

func (p *parser) skipNewlines() {
	for p.at(lexer.NEWLINE) {
		p.advance()
	}
}

// ---------- program ----------

func (p *parser) program() (*ast.Program, error) {
	prog := &ast.Program{}
	for {
		p.skipNewlines()
		if p.at(lexer.EOF) {
			return prog, nil
		}
		switch p.cur().Type {
		case lexer.STRUCT:
			s, err := p.structDecl()
			if err != nil {
				return nil, err
			}
			prog.Structs = append(prog.Structs, s)
		case lexer.DEF:
			f, err := p.funcDecl("")
			if err != nil {
				return nil, err
			}
			prog.Funcs = append(prog.Funcs, f)
		default:
			s, err := p.statement()
			if err != nil {
				return nil, err
			}
			prog.Main = append(prog.Main, s)
		}
	}
}

func (p *parser) structDecl() (*ast.StructDecl, error) {
	pos := p.pos_()
	p.advance() // struct
	name, err := p.expect(lexer.CONST)
	if err != nil {
		return nil, err
	}
	d := &ast.StructDecl{Pos: pos, Name: name.Lit}
	for {
		p.skipNewlines()
		if p.at(lexer.EOF) {
			return nil, p.errf("expected end to close struct %s", d.Name)
		}
		if p.accept(lexer.END) {
			return d, nil
		}
		if p.at(lexer.DEF) {
			m, err := p.funcDecl(d.Name)
			if err != nil {
				return nil, err
			}
			d.Methods = append(d.Methods, m)
			continue
		}
		fpos := p.pos_()
		fname, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.COLON); err != nil {
			return nil, err
		}
		ft, err := p.typeNode()
		if err != nil {
			return nil, err
		}
		d.Fields = append(d.Fields, &ast.Field{Pos: fpos, Name: fname.Lit, Type: ft})
	}
}

func (p *parser) funcDecl(recv string) (*ast.FuncDecl, error) {
	pos := p.pos_()
	p.advance() // def
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}
	fn := &ast.FuncDecl{Pos: pos, Name: name.Lit, Recv: recv}
	if p.accept(lexer.LPAREN) {
		for {
			p.skipNewlines()
			if p.accept(lexer.RPAREN) {
				break
			}
			ppos := p.pos_()
			pname, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.COLON); err != nil {
				return nil, err
			}
			pt, err := p.typeNode()
			if err != nil {
				return nil, err
			}
			fn.Params = append(fn.Params, &ast.Param{Pos: ppos, Name: pname.Lit, Type: pt})
			p.skipNewlines()
			if p.accept(lexer.COMMA) {
				continue
			}
			if _, err := p.expect(lexer.RPAREN); err != nil {
				return nil, err
			}
			break
		}
	}
	if p.accept(lexer.COLON) {
		rt, err := p.typeNode()
		if err != nil {
			return nil, err
		}
		fn.Ret = rt
	}
	body, err := p.body(lexer.END)
	if err != nil {
		return nil, err
	}
	fn.Body = body
	if _, err := p.expect(lexer.END); err != nil {
		return nil, err
	}
	return fn, nil
}

// typeNode parses Int, Array(Int), Channel(Point), Point.
func (p *parser) typeNode() (*ast.TypeNode, error) {
	pos := p.pos_()
	name, err := p.expect(lexer.CONST)
	if err != nil {
		return nil, err
	}
	t := &ast.TypeNode{Pos: pos, Name: name.Lit}
	if p.accept(lexer.LPAREN) {
		for {
			arg, err := p.typeNode()
			if err != nil {
				return nil, err
			}
			t.Args = append(t.Args, arg)
			if p.accept(lexer.COMMA) {
				continue
			}
			if _, err := p.expect(lexer.RPAREN); err != nil {
				return nil, err
			}
			break
		}
	}
	return t, nil
}

// body reads statements until one of the given terminators (not consumed).
func (p *parser) body(terms ...lexer.Type) ([]ast.Stmt, error) {
	var out []ast.Stmt
	for {
		p.skipNewlines()
		if p.at(lexer.EOF) {
			return out, nil
		}
		for _, t := range terms {
			if p.at(t) {
				return out, nil
			}
		}
		s, err := p.statement()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
}

// ---------- statements ----------

func (p *parser) statement() (ast.Stmt, error) {
	s, err := p.simpleStatement()
	if err != nil {
		return nil, err
	}
	// Trailing modifiers: expr if cond / expr unless cond / expr while cond
	for {
		switch p.cur().Type {
		case lexer.IF, lexer.UNLESS:
			neg := p.at(lexer.UNLESS)
			pos := p.pos_()
			p.advance()
			cond, err := p.expression()
			if err != nil {
				return nil, err
			}
			if neg {
				cond = &ast.UnaryExpr{Pos: pos, Op: "!", X: cond}
			}
			s = &ast.IfStmt{Pos: pos, Cond: cond, Then: []ast.Stmt{s}}
		case lexer.WHILE:
			pos := p.pos_()
			p.advance()
			cond, err := p.expression()
			if err != nil {
				return nil, err
			}
			s = &ast.WhileStmt{Pos: pos, Cond: cond, Body: []ast.Stmt{s}}
		default:
			if err := p.endOfStatement(); err != nil {
				return nil, err
			}
			return s, nil
		}
	}
}

func (p *parser) endOfStatement() error {
	switch p.cur().Type {
	case lexer.NEWLINE, lexer.EOF, lexer.END, lexer.ELSE, lexer.ELSIF, lexer.RBRACE:
		return nil
	}
	return p.errf("unexpected %s after statement", p.cur())
}

func (p *parser) simpleStatement() (ast.Stmt, error) {
	pos := p.pos_()
	switch p.cur().Type {
	case lexer.IF, lexer.UNLESS:
		return p.ifStatement()
	case lexer.WHILE:
		p.advance()
		cond, err := p.expression()
		if err != nil {
			return nil, err
		}
		p.accept(lexer.DO)
		body, err := p.body(lexer.END)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.END); err != nil {
			return nil, err
		}
		return &ast.WhileStmt{Pos: pos, Cond: cond, Body: body}, nil
	case lexer.RETURN:
		p.advance()
		st := &ast.ReturnStmt{Pos: pos}
		switch p.cur().Type {
		case lexer.NEWLINE, lexer.EOF, lexer.END, lexer.IF, lexer.UNLESS:
		default:
			v, err := p.expression()
			if err != nil {
				return nil, err
			}
			st.Value = v
		}
		return st, nil
	case lexer.BREAK:
		p.advance()
		return &ast.BreakStmt{Pos: pos}, nil
	case lexer.NEXT:
		p.advance()
		return &ast.NextStmt{Pos: pos}, nil
	case lexer.SPAWN:
		p.advance()
		blk, err := p.block()
		if err != nil {
			return nil, err
		}
		return &ast.SpawnStmt{Pos: pos, Body: blk.Body}, nil
	case lexer.DEFER:
		p.advance()
		blk, err := p.block()
		if err != nil {
			return nil, err
		}
		return &ast.DeferStmt{Pos: pos, Body: blk.Body}, nil
	case lexer.DEF:
		return nil, p.errf("def is only allowed at the top level or inside a struct")
	case lexer.STRUCT:
		return nil, p.errf("struct is only allowed at the top level")
	}

	// x : Int = 0
	if p.at(lexer.IDENT) && p.peek().Type == lexer.COLON {
		name := p.advance().Lit
		p.advance() // :
		t, err := p.typeNode()
		if err != nil {
			return nil, err
		}
		d := &ast.VarDecl{Pos: pos, Name: name, Type: t}
		if p.accept(lexer.ASSIGN) {
			v, err := p.expression()
			if err != nil {
				return nil, err
			}
			d.Value = v
		}
		return d, nil
	}

	x, err := p.expression()
	if err != nil {
		return nil, err
	}
	if p.at(lexer.ASSIGN) {
		p.advance()
		v, err := p.expression()
		if err != nil {
			return nil, err
		}
		switch x.(type) {
		case *ast.Ident, *ast.IVar, *ast.FieldExpr, *ast.IndexExpr:
		default:
			return nil, p.errf("cannot assign to this expression")
		}
		return &ast.AssignStmt{Pos: pos, Target: x, Value: v}, nil
	}
	return &ast.ExprStmt{Pos: pos, X: x}, nil
}

func (p *parser) ifStatement() (ast.Stmt, error) {
	pos := p.pos_()
	neg := p.at(lexer.UNLESS)
	p.advance()
	cond, err := p.expression()
	if err != nil {
		return nil, err
	}
	if neg {
		cond = &ast.UnaryExpr{Pos: pos, Op: "!", X: cond}
	}
	p.accept(lexer.THEN)
	then, err := p.body(lexer.ELSIF, lexer.ELSE, lexer.END)
	if err != nil {
		return nil, err
	}
	st := &ast.IfStmt{Pos: pos, Cond: cond, Then: then}
	for p.at(lexer.ELSIF) {
		bpos := p.pos_()
		p.advance()
		c, err := p.expression()
		if err != nil {
			return nil, err
		}
		p.accept(lexer.THEN)
		b, err := p.body(lexer.ELSIF, lexer.ELSE, lexer.END)
		if err != nil {
			return nil, err
		}
		st.Elifs = append(st.Elifs, &ast.Branch{Pos: bpos, Cond: c, Body: b})
	}
	if p.accept(lexer.ELSE) {
		b, err := p.body(lexer.END)
		if err != nil {
			return nil, err
		}
		st.Else = b
		if b == nil {
			st.Else = []ast.Stmt{}
		}
	}
	if _, err := p.expect(lexer.END); err != nil {
		return nil, err
	}
	return st, nil
}

// ---------- expressions ----------

func precedence(t lexer.Type) int {
	switch t {
	case lexer.OROR, lexer.OR:
		return 1
	case lexer.ANDAND, lexer.AND:
		return 2
	case lexer.EQ, lexer.NEQ, lexer.LT, lexer.LTE, lexer.GT, lexer.GTE:
		return 3
	case lexer.DOTDOT, lexer.DOTDOTDOT:
		return 4
	case lexer.SHL:
		return 5
	case lexer.PLUS, lexer.MINUS:
		return 6
	case lexer.STAR, lexer.SLASH, lexer.PERCENT:
		return 7
	case lexer.POW:
		return 8
	}
	return 0
}

var opText = map[lexer.Type]string{
	lexer.OROR: "||", lexer.OR: "||", lexer.ANDAND: "&&", lexer.AND: "&&",
	lexer.EQ: "==", lexer.NEQ: "!=", lexer.LT: "<", lexer.LTE: "<=",
	lexer.GT: ">", lexer.GTE: ">=", lexer.SHL: "<<", lexer.PLUS: "+",
	lexer.MINUS: "-", lexer.STAR: "*", lexer.SLASH: "/", lexer.PERCENT: "%",
	lexer.POW: "**",
}

func (p *parser) expression() (ast.Expr, error) { return p.binary(1) }

func (p *parser) binary(minPrec int) (ast.Expr, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.cur().Type
		prec := precedence(t)
		if prec == 0 || prec < minPrec {
			return left, nil
		}
		pos := p.pos_()
		p.advance()
		next := prec + 1
		if t == lexer.POW {
			next = prec // right associative
		}
		right, err := p.binary(next)
		if err != nil {
			return nil, err
		}
		if t == lexer.DOTDOT || t == lexer.DOTDOTDOT {
			left = &ast.RangeExpr{Pos: pos, Lo: left, Hi: right, Exclusive: t == lexer.DOTDOTDOT}
			continue
		}
		left = &ast.BinaryExpr{Pos: pos, Op: opText[t], L: left, R: right}
	}
}

func (p *parser) unary() (ast.Expr, error) {
	pos := p.pos_()
	switch p.cur().Type {
	case lexer.MINUS:
		p.advance()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Pos: pos, Op: "-", X: x}, nil
	case lexer.BANG, lexer.NOT:
		p.advance()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Pos: pos, Op: "!", X: x}, nil
	}
	x, err := p.primary()
	if err != nil {
		return nil, err
	}
	return p.postfix(x)
}

func (p *parser) postfix(x ast.Expr) (ast.Expr, error) {
	for {
		pos := p.pos_()
		switch p.cur().Type {
		case lexer.DOT:
			p.advance()
			name, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, err
			}
			if name.Lit == "new" {
				t, err := asType(x)
				if err != nil {
					return nil, p.errf("%s", err.Error())
				}
				n := &ast.NewExpr{Pos: pos, Type: t}
				if p.accept(lexer.LPAREN) {
					args, kw, err := p.args()
					if err != nil {
						return nil, err
					}
					n.Args, n.KwArgs = args, kw
				}
				x = n
				continue
			}
			call := &ast.MethodCall{Pos: pos, Recv: x, Name: name.Lit}
			if p.accept(lexer.LPAREN) {
				args, kw, err := p.args()
				if err != nil {
					return nil, err
				}
				call.Args, call.KwArgs, call.Parens = args, kw, true
			}
			if p.at(lexer.DO) || p.at(lexer.LBRACE) {
				blk, err := p.block()
				if err != nil {
					return nil, err
				}
				call.Block = blk
			}
			if !call.Parens && call.Block == nil {
				x = &ast.FieldExpr{Pos: pos, Recv: call.Recv, Name: call.Name}
			} else {
				x = call
			}
		case lexer.LBRACKET:
			p.advance()
			idx, err := p.expression()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.RBRACKET); err != nil {
				return nil, err
			}
			x = &ast.IndexExpr{Pos: pos, Recv: x, Index: idx}
		case lexer.LPAREN:
			id, ok := x.(*ast.Ident)
			if !ok {
				return x, nil
			}
			p.advance()
			args, _, err := p.args()
			if err != nil {
				return nil, err
			}
			call := &ast.CallExpr{Pos: id.Position(), Name: id.Name, Args: args}
			if p.at(lexer.DO) || p.at(lexer.LBRACE) {
				blk, err := p.block()
				if err != nil {
					return nil, err
				}
				call.Block = blk
			}
			x = call
		default:
			return x, nil
		}
	}
}

// asType converts an expression used as a receiver of .new into a type.
func asType(x ast.Expr) (*ast.TypeNode, error) {
	switch v := x.(type) {
	case *ast.TypeRef:
		return v.Type, nil
	case *ast.Ident:
		return &ast.TypeNode{Pos: v.Pos, Name: v.Name}, nil
	}
	return nil, fmt.Errorf("new can only be called on a type name")
}

// args parses a call's argument list; the '(' is already consumed.
func (p *parser) args() ([]ast.Expr, []*ast.KeyVal, error) {
	var args []ast.Expr
	var kw []*ast.KeyVal
	for {
		p.skipNewlines()
		if p.accept(lexer.RPAREN) {
			return args, kw, nil
		}
		if p.at(lexer.IDENT) && p.peek().Type == lexer.COLON {
			pos := p.pos_()
			name := p.advance().Lit
			p.advance()
			v, err := p.expression()
			if err != nil {
				return nil, nil, err
			}
			kw = append(kw, &ast.KeyVal{Pos: pos, Name: name, Value: v})
		} else {
			v, err := p.expression()
			if err != nil {
				return nil, nil, err
			}
			args = append(args, v)
		}
		p.skipNewlines()
		if p.accept(lexer.COMMA) {
			continue
		}
		p.skipNewlines()
		if _, err := p.expect(lexer.RPAREN); err != nil {
			return nil, nil, err
		}
		return args, kw, nil
	}
}

func (p *parser) block() (*ast.Block, error) {
	pos := p.pos_()
	brace := p.at(lexer.LBRACE)
	if !brace && !p.at(lexer.DO) {
		return nil, p.errf("expected do or {, found %s", p.cur())
	}
	p.advance()
	blk := &ast.Block{Pos: pos}
	p.skipNewlines()
	if p.accept(lexer.PIPE) {
		for {
			name, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, err
			}
			blk.Params = append(blk.Params, name.Lit)
			if p.accept(lexer.COMMA) {
				continue
			}
			if _, err := p.expect(lexer.PIPE); err != nil {
				return nil, err
			}
			break
		}
	}
	term := lexer.END
	if brace {
		term = lexer.RBRACE
	}
	body, err := p.body(term)
	if err != nil {
		return nil, err
	}
	blk.Body = body
	if _, err := p.expect(term); err != nil {
		return nil, err
	}
	return blk, nil
}

// commands may be called without parentheses: puts "hi"
var commands = map[string]bool{"puts": true, "print": true, "p": true}

// startsExpr reports whether t could begin an argument to a paren-less command.
func startsExpr(t lexer.Type) bool {
	switch t {
	case lexer.INT, lexer.FLOAT, lexer.STRING, lexer.IDENT, lexer.CONST,
		lexer.IVAR, lexer.SELF, lexer.TRUE, lexer.FALSE, lexer.NIL,
		lexer.LBRACKET, lexer.LPAREN, lexer.MINUS, lexer.BANG, lexer.NOT:
		return true
	}
	return false
}

func (p *parser) primary() (ast.Expr, error) {
	t := p.cur()
	pos := ast.Pos{Line: t.Line, Col: t.Col}
	switch t.Type {
	case lexer.INT:
		p.advance()
		return &ast.IntLit{Pos: pos, Val: t.Lit}, nil
	case lexer.FLOAT:
		p.advance()
		return &ast.FloatLit{Pos: pos, Val: t.Lit}, nil
	case lexer.STRING:
		p.advance()
		return p.stringLit(t, pos)
	case lexer.TRUE, lexer.FALSE:
		p.advance()
		return &ast.BoolLit{Pos: pos, Val: t.Type == lexer.TRUE}, nil
	case lexer.NIL:
		p.advance()
		return &ast.NilLit{Pos: pos}, nil
	case lexer.SELF:
		p.advance()
		return &ast.SelfExpr{Pos: pos}, nil
	case lexer.IVAR:
		p.advance()
		return &ast.IVar{Pos: pos, Name: t.Lit}, nil
	case lexer.LPAREN:
		p.advance()
		p.skipNewlines()
		x, err := p.expression()
		if err != nil {
			return nil, err
		}
		p.skipNewlines()
		if _, err := p.expect(lexer.RPAREN); err != nil {
			return nil, err
		}
		return x, nil
	case lexer.LBRACKET:
		p.advance()
		lit := &ast.ArrayLit{Pos: pos}
		for {
			p.skipNewlines()
			if p.accept(lexer.RBRACKET) {
				return lit, nil
			}
			e, err := p.expression()
			if err != nil {
				return nil, err
			}
			lit.Elems = append(lit.Elems, e)
			p.skipNewlines()
			if p.accept(lexer.COMMA) {
				continue
			}
			p.skipNewlines()
			if _, err := p.expect(lexer.RBRACKET); err != nil {
				return nil, err
			}
			return lit, nil
		}
	case lexer.CONST:
		p.advance()
		name := t.Lit
		if p.at(lexer.LPAREN) && (name == "Channel" || name == "Array") {
			p.advance()
			tn := &ast.TypeNode{Pos: pos, Name: name}
			for {
				arg, err := p.typeNode()
				if err != nil {
					return nil, err
				}
				tn.Args = append(tn.Args, arg)
				if p.accept(lexer.COMMA) {
					continue
				}
				if _, err := p.expect(lexer.RPAREN); err != nil {
					return nil, err
				}
				break
			}
			return &ast.TypeRef{Pos: pos, Type: tn}, nil
		}
		return &ast.Ident{Pos: pos, Name: name}, nil
	case lexer.IDENT:
		p.advance()
		if commands[t.Lit] && startsExpr(p.cur().Type) {
			call := &ast.CallExpr{Pos: pos, Name: t.Lit}
			for {
				a, err := p.expression()
				if err != nil {
					return nil, err
				}
				call.Args = append(call.Args, a)
				if !p.accept(lexer.COMMA) {
					return call, nil
				}
			}
		}
		return &ast.Ident{Pos: pos, Name: t.Lit}, nil
	}
	return nil, p.errf("unexpected %s", t)
}

// stringLit re-parses the interpolated segments recorded by the lexer.
func (p *parser) stringLit(t lexer.Token, pos ast.Pos) (ast.Expr, error) {
	lit := &ast.StrLit{Pos: pos}
	for _, part := range t.Parts {
		if !part.IsExpr {
			lit.Parts = append(lit.Parts, ast.StrPart{Text: part.Text})
			continue
		}
		toks, err := lexer.Tokenize(p.file, part.Text)
		if err != nil {
			return nil, err
		}
		for i := range toks {
			if toks[i].Line == 1 {
				toks[i].Col += part.Col - 1
			}
			toks[i].Line += part.Line - 1
		}
		sub := &parser{file: p.file, toks: toks}
		sub.skipNewlines()
		x, err := sub.expression()
		if err != nil {
			return nil, err
		}
		lit.Parts = append(lit.Parts, ast.StrPart{X: x})
	}
	return lit, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
