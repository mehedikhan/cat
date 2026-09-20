# How cat works

This document explains the compiler: what each stage does, which problems were
hard, and why they were solved the way they were. For the language itself, see
[README.md](README.md).

## The premise

cat is "Ruby's syntax, Go's semantics." Go's semantics means goroutines,
channels, static types, a garbage collector and native binaries. Writing those
means writing a scheduler, a GC, a type checker and a linker.

So cat doesn't. **cat is a source-to-source compiler: it emits Go and hands the
result to the Go toolchain.** Every hard runtime problem becomes someone else's
solved problem, and cat only has to be a front end.

The cost of that choice shows up in one place — errors. The Go compiler reports
problems against generated Go that the programmer never wrote. Most of the
engineering below is about paying that cost down.

## The pipeline

```
   hello.cat
       │
       ▼
┌──────────────┐  Tokenize()        tokens; newline-terminated statements,
│    lexer     │                    #{} segments captured as raw source
└──────┬───────┘
       ▼
┌──────────────┐  Parse()           recursive descent for statements,
│    parser    │                    precedence climbing for expressions
└──────┬───────┘
       ▼    *ast.Program
┌──────────────┐  Generate()        Go source text  +  goLine → catLine map
│   codegen    │
└──────┬───────┘
       ▼
┌──────────────┐  Build()           temp module → `go build` → rewrite errors
│    driver    │                    through the line map
└──────┬───────┘
       ▼
  native binary
```

Four packages, one direction, no cycles. `internal/ast` sits to the side as the
shared vocabulary between parser and codegen.

---

## Stage 1 — Lexer

`internal/lexer/lexer.go`. Input is source text, output is a flat `[]Token`.
Lexing happens up front rather than on demand, because the newline rule needs to
look at the previously *emitted* token.

### Newlines are statement terminators — sometimes

Ruby ends statements at newlines, but a line ending mid-expression continues:

```ruby
total = 1 +
        2
```

The rule is a lookbehind: a newline is dropped if the last emitted token cannot
end a statement. That set is `noTerminate` (`lexer.go:141`) — every binary and
unary operator, plus `,` `.` `(` `[` `{` `|` `:` `do` `then` `else`, and NEWLINE
itself so blank lines collapse.

```go
if t.Type == NEWLINE {
    if len(l.toks) == 0 || noTerminate[l.toks[len(l.toks)-1].Type] {
        continue        // a continuation, not a terminator
    }
}
```

Leading-position tokens like `)` and `end` are deliberately absent, so
`end\n` terminates. A `\` before a newline is also swallowed, as an escape hatch.

### `empty?` and `save!` are names, `x != y` is not

Ruby lets method names end in `?` or `!`. Naively consuming a trailing `!`
breaks `x != y`. One character of lookahead settles it (`lexer.go:310`):

```go
if l.at(0) == '?' || (l.at(0) == '!' && l.at(1) != '=') {
    l.advance()
}
```

Because the check runs immediately after the identifier characters, with no
whitespace skip, `x ?` is never absorbed — only `x?`.

### `1..5` is a range, not a malformed float

The number scanner (`lexer.go:287`) reads digits, then only treats `.` as a
decimal point if a **digit** follows. `1..5` therefore lexes as `INT DOTDOT INT`
rather than `1.` followed by garbage. Underscores are stripped, so `1_000_000`
works.

### String interpolation is deferred, not parsed

`"worker #{id} of #{n}"` could be handled with a lexer that recursively re-enters
expression mode. Instead the lexer records *segments* and moves on
(`lexer.go:330`): literal text, or the raw source between `#{` and its matching
`}`, tracked by brace depth.

```go
type Part struct {
    IsExpr bool
    Text   string
    Line, Col int      // where the segment started, for error positions
}
```

The parser re-lexes and re-parses each expression segment later
(`parser.go:826`), shifting the resulting token positions by the segment's
origin so a syntax error inside `#{}` still reports the right line and column.
The tradeoff: a `}` inside a nested string literal inside an interpolation ends
the segment early. Acceptable for now, and localized to one function when it
needs fixing.

---

## Stage 2 — Parser

`internal/parser/parser.go`. Statements are recursive descent; expressions use
precedence climbing.

### Precedence

`precedence()` (`parser.go:453`) is the whole table:

| Level | Operators |
|---|---|
| 1 | `\|\|` `or` |
| 2 | `&&` `and` |
| 3 | `==` `!=` `<` `<=` `>` `>=` |
| 4 | `..` `...` |
| 5 | `<<` |
| 6 | `+` `-` |
| 7 | `*` `/` `%` |
| 8 | `**` (right associative) |

Ranges are parsed as a binary operator and then rewritten into a `RangeExpr`
node, which keeps `(1..n).each` falling out of the normal expression path with
no special case. `**` recurses at its own level instead of one above, which is
how right associativity is expressed in a precedence-climbing loop.

### Postfix chains

`postfix()` (`parser.go:539`) loops over `.name`, `(args)` and `[index]` so
`a.b(c)[0].d` parses without special cases. Two decisions live here:

- **`.name` with no parentheses and no block becomes a `FieldExpr`**, not a
  call. Codegen decides later whether it is a field access or a Ruby-style
  paren-less method call, because only codegen knows the struct declarations.
- **`.new` is intercepted** and turned into a `NewExpr` carrying a *type*, so
  `Point.new(...)`, `Channel(Int).new(8)` and `Array(String).new` all take one
  path. `asType()` converts the receiver expression into a type node.

### Blocks

`block()` (`parser.go:668`) accepts both `do |x| ... end` and `{ |x| ... }`.
The brace form is unambiguous only because cat has no hash literals yet — the
day it gets them, this is the function that has to disambiguate.

### Statement modifiers

`statement()` (`parser.go:261`) parses a simple statement and then loops on
trailing `if` / `unless` / `while`, wrapping what it already has:

```ruby
puts "negative" if n < 0     # parses as a full IfStmt
```

`unless` is not a distinct node — the condition is wrapped in a `!` and it
becomes an ordinary `IfStmt`. Same for the `unless` block form. Fewer node
types, fewer codegen cases.

### Paren-less calls: the deliberate cheat

Ruby's `f -x` is genuinely ambiguous — `f(-x)` or `f - x`? Resolving it needs
whitespace-sensitive lexing and knowledge of what is a method. cat v0.1 declines
the problem: only `puts`, `print` and `p` may be called bare (`commands`,
`parser.go:709`), and only when the next token can start an expression
(`startsExpr`). Everything else needs parentheses.

This is the single biggest gap between cat and real Ruby syntax, and it is
isolated to those two declarations.

---

## Stage 3 — Codegen

`internal/codegen/codegen.go`. Walks the AST and writes Go source **as text**,
with hand-managed indentation.

### Why text, and not go/ast

The obvious approach is to build a `go/ast` tree and print it with `go/printer`.
cat doesn't, for one reason: the line map.

Every emitted line must be recorded against the cat line it came from. Go's own
mechanism for this is the `//line file:line` directive, but a `//line` comment is
only honored at column 1, and gofmt indents comments inside function bodies —
which silently breaks it everywhere except top level.

So the generator owns its own output formatting and records positions as it
writes (`codegen.go:104`):

```go
func (g *gen) emit(pos ast.Pos, s string) {
    g.buf.WriteString(strings.Repeat("\t", g.indent))
    g.buf.WriteString(s)
    g.buf.WriteByte('\n')
    g.line++
    if pos.Line > 0 {
        g.lineMap[g.line] = pos.Line
    }
}
```

Imports aren't known until the body is generated, so the header is written last
and the whole map is shifted by the header's line count. `go/format` is still
used — but only by `catc emit`, where line numbers don't matter.

### The mapping

| cat | Go |
|---|---|
| `x = 1` (first time) | `x := 1` followed by `_ = x` |
| `x : Int = 1` | `var x int = 1` |
| `"a#{b}c"` | `fmt.Sprintf("a%vc", b)` |
| `puts e` | `fmt.Println(e)` |
| `struct Point` | `type Point struct` |
| `def m` inside a struct | `func (self *Point) m()` |
| `@x` | `self.x` |
| `Point.new(x: 1.0)` | `Point{x: 1.0}` |
| `Channel(Int).new(8)` | `make(chan int, 8)` |
| `spawn do ... end` | `go func() { ... }()` |
| `defer do ... end` | `defer func() { ... }()` |
| `ch.receive` | `(<-ch)` |
| `ch.close` | `close(ch)` |
| `xs.size` | `len(xs)` |
| `next` / `break` | `continue` / `break` |
| `a ** b` | `math.Pow(float64(a), float64(b))` |

Imports are collected on demand — `g.need("fmt")` is called at the point of use,
so nothing unused is ever emitted.

### Hard mapping 1: blocks become loops, not closures

`xs.each do |x|` could become `xs.each(func(x T) { ... })`. cat lowers it to a
`for` range loop instead (`loop()`, `codegen.go:494`):

```ruby
xs.each do |x| ... end     →  for _, x := range xs { ... }
ch.each do |v| ... end     →  for v := range ch { ... }
(1...n).each do |i| ... end →  for i := 0; i < n; i++ { ... }
n.times do |i| ... end     →  for i := 0; i < n; i++ { ... }
```

The reason is `break` and `next`. In a closure they cannot work — `break` out of
a function literal is not a thing in Go, and a Ruby programmer expects both to
behave exactly as they do in a loop. Unrolling gets that for free, and avoids a
closure call per element.

One consequence worth knowing: the emitted module declares `go 1.22`
(`minGo`, `toolchain.go:13`), and `catc` refuses to build with an older
toolchain rather than producing a subtly wrong program. Under 1.21 semantics a loop variable is shared across
iterations, so `3.times { |i| spawn { worker(i) } }` handed every goroutine the
same `i`, and all three reported the same worker id. Go 1.22's per-iteration
loop variables match Ruby's per-iteration block binding exactly.

### Hard mapping 2: `<<` is two different operators

`ch << v` is a channel send; `xs << v` is an append. Go spells them completely
differently, and cat has no type checker to tell them apart.

The compiler carries a *kind* for each variable — `chan`, `slice`, or unknown —
in a scope stack, populated from typed declarations, parameter types,
`Channel(T).new` and array literals. `shiftStmt` (`codegen.go:480`) dispatches
on it:

```go
switch g.kindOf(v.L) {
case kindChan:  g.emit(v.Pos, lhs+" <- "+rhs)
case kindSlice: g.emit(v.Pos, lhs+" = append("+lhs+", "+rhs+")")
default:        g.errf(v.Pos, "cannot tell whether << is a channel send or "+
                    "an array push; give the variable a type, ...")
}
```

This is a keyhole type inference — the smallest amount of type knowledge that
makes the syntax work. When it can't tell, it says so and names the fix. The
same kind information decides whether `.each` ranges over a channel (one loop
variable) or a slice (two).

### Hard mapping 3: implicit returns and Go's terminating statement rule

Ruby returns the last expression. Go requires a terminating statement. So
`funcBody` (`codegen.go:321`) rewrites a trailing expression into a `return`:

```ruby
def add(a : Int, b : Int) : Int
  a + b                  →   return (a + b)
end
```

But cat code often returns from inside branches instead, and Go rejects that as
"missing return" even when every path returns. Rather than implement flow
analysis, codegen appends an unreachable `panic`:

```go
func sign(n int) string {
    if n < 0 {
        return "neg"
    } else {
        return "pos"
    }
    panic("cat: sign ended without returning a value")
}
```

If a path genuinely falls through, the message names the function — a real
runtime error rather than a confusing compile error about generated code.

### Hard mapping 4: `a.to_str` — field or method?

Ruby calls methods without parentheses. Go does not. When the parser sees
`a.to_str` it cannot know which, so it emits a `FieldExpr` and lets codegen
decide (`field()`, `codegen.go:677`). Codegen has indexed every struct's field
and method names:

```go
if g.methods[v.Name] && !g.fields[v.Name] {
    return g.expr(v.Recv) + "." + mangle(v.Name) + "()"
}
```

A bare `.name` is a field unless *only* a method has that name. If a field
anywhere shares the name, field access wins and the call needs parentheses.

### Name mangling

Two name spaces have to be reconciled (`mangle()`, `codegen.go:170`):

- `empty?` → `empty_p`, `save!` → `save_bang`
- Go keywords and predeclared identifiers get a `_` suffix, so a cat variable
  named `type`, `range`, `select`, `len` or `string` is legal.

### `_ = x`

Go rejects unused local variables. Ruby programmers do not expect that, and a
throwaway assignment is normal style. So every new local and every block
parameter is followed by `_ = name`. It's noisy in the emitted Go and it does
give up a genuinely useful check — a deliberate trade in favor of the language
feeling like Ruby.

---

## Stage 4 — Driver

`internal/driver/driver.go`. Compiles, builds, runs, and — the important part —
translates errors.

`Build()` writes a throwaway module to a temp directory:

```
/tmp/catbuildXXXX/
    go.mod      module catprog / go 1.22
    main.go     the generated source
```

then shells out to `go build -o <abs path> .` and deletes the directory. `Run()`
builds into a temp path and execs the binary with stdio wired through, so the
program's exit code is cat's exit code.

### Error rewriting

`rewrite()` (`driver.go:119`) is what makes the whole transpiler approach
tolerable. Any `main.go:37:5:` prefix in toolchain output is looked up in the
line map and replaced with the cat file and line; the `# catprog` package banner
is dropped:

```
$ catc run bad.cat
bad.cat:7: cannot use y (variable of type string) as int value in argument to add
bad.cat:9: undefined: undefined_thing
```

A generated line with no mapping — header lines, the synthesized `panic` —
degrades to `bad.cat (generated line 37):` rather than lying about a position.

The column is dropped on purpose. Go's column refers to generated text; a cat
column would be a guess. A correct line with no column beats a confident wrong
column.

---

## Where types are checked

Nowhere in cat. That is the defining architectural choice of this version.

The Go compiler is the type checker, reached through generated code, with its
diagnostics mapped back. cat's own static checks are only what codegen can see:
unknown types, unknown struct names, wrong arity in `T.new`, `<<` on a variable
of unknown kind, a block on a method that doesn't take one.

What this buys: no type system to build, full Go type inference and generics
underneath, and correctness guaranteed by a compiler that is already right.

What it costs:

- Error *wording* is Go's, so it mentions `int` rather than `Int`, and
  `[]string` rather than `Array(String)`.
- Errors arrive from the build step, not from cat, so they show up all at once
  at the end.
- Some cat-level mistakes surface as confusing Go-level ones.

A real type checker is the obvious next major piece of work. Its natural shape
here is `go/types` driven directly over the generated AST — the same checker,
but queried programmatically so the messages can be rewritten into cat's
vocabulary instead of scraped from stderr.

---

## A full trace

`examples/workers.cat`:

```ruby
def worker(id : Int, jobs : Channel(Int), results : Channel(String))
  jobs.each do |job|
    results << "worker #{id} finished job #{job}"
  end
end

jobs = Channel(Int).new(16)
3.times do |i|
  spawn do
    worker(i, jobs, results)
  end
end
```

**Lexer.** `Channel` lexes as `CONST` because it starts uppercase. The newline
after `do |job|` is dropped (`noTerminate[DO]`). The string becomes four parts:
`"worker "`, expr `id`, `" finished job "`, expr `job`.

**Parser.** `jobs : Channel(Int)` is a typed parameter. `jobs.each do |job|` is a
`MethodCall` with a `Block`. `results << "..."` is a `BinaryExpr` with op `<<` —
at this point nothing knows it is a channel send. `Channel(Int).new(16)` is a
`TypeRef` that `postfix` converts to a `NewExpr`.

**Codegen.** The parameter type registers `results` with kind `chan`, so `<<`
lowers to a send. `jobs.each` sees kind `chan` and emits a one-variable range.
`3.times do |i|` becomes a counted loop; `spawn` becomes `go func() { ... }()`,
capturing the per-iteration `i`. `fmt` is requested by the interpolation and
appears in the import block.

**Output:**

```go
func worker(id int, jobs chan int, results chan string) {
	for job := range jobs {
		_ = job
		results <- fmt.Sprintf("worker %v finished job %v", id, job)
	}
}

func main() {
	jobs := make(chan int, 16)
	_ = jobs
	for i := 0; i < 3; i++ {
		_ = i
		go func() {
			worker(i, jobs, results)
		}()
	}
	...
}
```

**Driver.** Written to a temp module, built, executed. Any complaint from `go
build` comes back addressed to `workers.cat`.

Run `catc emit <file>` on anything to see this for yourself — it is the single
most useful debugging tool in the project.

---

## Adding a feature

The pipeline is short enough that most features touch a predictable set of
places. For a new method like `xs.first`:

1. **codegen only** — add a case to `methodCall()` or `field()`. Most built-ins
   need nothing else.

For a new keyword like `case`/`when`:

1. `lexer.go` — add the token constant, `names`, and the `keywords` map; decide
   whether a newline after it continues the line (`noTerminate`).
2. `ast.go` — add the node and its `stmt()` or `expr()` marker.
3. `parser.go` — parse it; add to `precedence` if it is an operator.
4. `codegen.go` — lower it; call `g.need()` for any import it requires.
5. `examples/` and a test — the driver tests compile and run every example, so
   a new example is a regression test.

The compiler is deliberately small: roughly 400 lines of lexer, 850 of parser,
800 of codegen, 150 of driver. Every stage fits in one file, on purpose.

## Known sharp edges

- A `}` inside a nested string inside `#{}` ends the interpolation early.
- Codegen wraps every binary expression in parentheses. Correct, but the emitted
  Go reads `((a * a) + (b * b))`.
- Struct methods use pointer receivers, so a method call on a non-addressable
  value — `Point.new(1.0, 2.0).dist(q)` — is a Go error.
- The kind tracker knows `chan` and `slice` and nothing else, so `<<` on a value
  returned from a function can't be resolved.
- One file per program. There is no import mechanism yet, and adding one means
  deciding whether cat modules map to Go packages or get concatenated.
