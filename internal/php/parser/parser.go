package parser

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

const eof = token.Invalid

type parser struct {
	src       string
	version   token.Version
	toks      []lexer.Token
	docs      []ast.Span
	pos       int
	prevEnd   uint32
	emptyAt   uint32
	errors    []ast.Error
	truncated bool

	parenthesized ast.Expr
	depth         int
}

const maxDepth = 4096

func Parse(src string, version token.Version) *ast.File {
	return parse(src, version, lexer.Lex(src, version))
}

func ParseCode(src string, version token.Version) *ast.File {
	return parse(src, version, lexer.LexCode(src, version))
}

func parse(src string, version token.Version, all []lexer.Token) *ast.File {
	p := &parser{src: src, version: version, toks: make([]lexer.Token, 0, len(all)/2+1), docs: make([]ast.Span, 0, len(all)/2+1)}
	var doc ast.Span
	for _, tok := range all {
		switch tok.Kind {
		case token.Whitespace, token.OpenTag:
		case token.Comment:
			p.unterminated(tok)
		case token.DocComment:
			p.unterminated(tok)
			doc = ast.Span{Start: tok.Start, End: tok.End}
		default:
			p.toks = append(p.toks, tok)
			p.docs = append(p.docs, doc)
			doc = ast.Span{}
		}
	}
	file := &ast.File{Span: ast.Span{End: uint32(len(src))}}
	p.guard(func() { p.file(file) })
	slices.SortStableFunc(p.errors, func(a, b ast.Error) int { return cmp.Compare(a.Start, b.Start) })
	file.Errors = p.errors
	return file
}

func (p *parser) file(file *ast.File) {
	for k := p.peek(); k != eof; k = p.peek() {
		start, before := p.start(), p.pos
		if k == token.RightBrace {
			p.unexpected("expected a statement")
			p.next()
			file.Stmts = append(file.Stmts, &ast.BadStmt{Span: p.span(start)})
			continue
		}
		if s := p.statement(); s != nil {
			file.Stmts = append(file.Stmts, s)
		}
		p.ensureProgress(before)
	}
}

func (p *parser) guard(run func()) {
	defer func() {
		if r := recover(); r != nil {
			at := p.start()
			p.errors = append(p.errors, ast.Error{Span: ast.Span{Start: at, End: at}, Message: fmt.Sprintf("internal parser error: %v", r)})
		}
	}()
	run()
}

func (p *parser) enter() bool {
	p.depth++
	if p.depth <= maxDepth {
		return true
	}
	t := p.tok()
	p.unterminatedAt(ast.Span{Start: t.Start, End: t.End}, "nesting is too deep to parse")
	p.pos = len(p.toks)
	return false
}

func isEOF(token.Kind) bool { return false }

func (p *parser) unterminated(comment lexer.Token) {
	text := comment.Text(p.src)
	if strings.HasPrefix(text, "/*") && (len(text) < 4 || !strings.HasSuffix(text, "*/")) {
		p.unterminatedAt(ast.Span{Start: comment.Start, End: comment.Start + 2}, "unterminated comment")
	}
}

func (p *parser) unterminatedAt(span ast.Span, message string) {
	p.errorAt(span, message)
	p.truncated = true
}

type mark struct {
	pos       int
	errors    int
	prevEnd   uint32
	emptyAt   uint32
	truncated bool
}

func (p *parser) mark() mark {
	return mark{pos: p.pos, errors: len(p.errors), prevEnd: p.prevEnd, emptyAt: p.emptyAt, truncated: p.truncated}
}

func (p *parser) reset(m mark) {
	p.pos, p.errors, p.prevEnd, p.emptyAt, p.truncated = m.pos, p.errors[:m.errors], m.prevEnd, m.emptyAt, m.truncated
}

func (p *parser) peek() token.Kind {
	return p.peekAt(0)
}

func (p *parser) peekAt(n int) token.Kind {
	if p.pos+n >= len(p.toks) {
		return eof
	}
	return p.toks[p.pos+n].Kind
}

func (p *parser) tok() lexer.Token {
	if p.pos >= len(p.toks) {
		end := uint32(len(p.src))
		return lexer.Token{Kind: eof, Start: end, End: end}
	}
	return p.toks[p.pos]
}

func (p *parser) start() uint32 {
	return p.tok().Start
}

func (p *parser) doc() ast.Span {
	return p.docSince(p.pos)
}

func (p *parser) docSince(from int) ast.Span {
	for i := min(p.pos, len(p.docs)-1); i >= from; i-- {
		if p.docs[i] != (ast.Span{}) {
			return p.docs[i]
		}
	}
	return ast.Span{}
}

func (p *parser) next() lexer.Token {
	t := p.tok()
	if p.pos < len(p.toks) {
		p.pos++
		p.prevEnd = t.End
	}
	return t
}

func (p *parser) span(start uint32) ast.Span {
	if start > p.prevEnd {
		p.emptyAt = max(p.emptyAt, start)
		return ast.Span{Start: start, End: start}
	}
	end := p.prevEnd
	if p.emptyAt > end {
		end = p.emptyAt
	}
	return ast.Span{Start: start, End: end}
}

func (p *parser) accept(k token.Kind) bool {
	if next := p.peek(); next != k && (k != token.Semicolon || next != token.CloseTag) {
		return false
	}
	p.next()
	return true
}

func (p *parser) expect(k token.Kind) bool {
	if p.accept(k) {
		return true
	}
	p.unexpected("expected " + kindName(k))
	return false
}

func kindName(k token.Kind) string {
	switch k {
	case token.Variable:
		return "a variable"
	case token.Identifier:
		return "an identifier"
	case token.NumString:
		return "a number"
	case token.EndHeredoc:
		return "the end of the heredoc"
	}
	return "'" + k.Text() + "'"
}

func (p *parser) unexpected(message string) {
	t := p.tok()
	found := "end of file"
	if t.Kind != eof {
		found = "'" + t.Text(p.src) + "'"
	}
	if message != "" {
		message = ", " + message
	}
	p.errorAt(ast.Span{Start: t.Start, End: t.End}, "syntax error, unexpected "+found+message)
}

func (p *parser) errorAt(span ast.Span, message string) {
	if n := len(p.errors); n > 0 && p.errors[n-1].Start == span.Start || p.truncated && span.Start == uint32(len(p.src)) {
		return
	}
	p.errors = append(p.errors, ast.Error{Span: span, Message: message})
}

func (p *parser) requires(v token.Version, start uint32, feature string) {
	if p.version < v {
		p.errorAt(p.span(start), feature+" requires PHP "+v.String())
	}
}

func (p *parser) ensureProgress(before int) {
	if p.pos == before {
		p.unexpected("")
		p.next()
	}
}

func (p *parser) skipTo(stop func(token.Kind) bool) {
	depth := 0
	for k := p.peek(); k != eof; k = p.peek() {
		if depth == 0 && (stop(k) || p.atStatementStart()) {
			return
		}
		switch k {
		case token.LeftBrace, token.CurlyOpen, token.DollarOpenCurlyBraces:
			depth++
		case token.RightBrace:
			if depth == 0 {
				return
			}
			depth--
		}
		p.next()
	}
}

func (p *parser) atStatementStart() bool {
	if p.pos > 0 {
		switch p.toks[p.pos-1].Kind {
		case token.DoubleColon, token.New:
			return false
		}
	}
	switch p.peek() {
	case token.If, token.ElseIf, token.Else, token.EndIf, token.While, token.EndWhile, token.Do, token.For, token.EndFor,
		token.Foreach, token.EndForeach, token.Switch, token.Case, token.Default, token.EndSwitch, token.Break, token.Continue,
		token.Return, token.Global, token.Echo, token.Unset, token.Try, token.Catch, token.Finally, token.Goto,
		token.Declare, token.EndDeclare, token.HaltCompiler, token.Namespace, token.Use, token.Const, token.Class,
		token.Interface, token.Trait, token.Enum, token.Abstract, token.Final, token.Public, token.Protected, token.Private,
		token.Var, token.InlineHTML, token.OpenTagWithEcho:
		return true
	case token.Function:
		return p.isFunctionDecl()
	case token.Readonly:
		return p.peekAt(1) != token.LeftParen
	}
	return false
}

func (p *parser) identifier() *ast.Identifier {
	if !isIdentifier(p.peek()) {
		p.unexpected("expected an identifier")
		return nil
	}
	t := p.next()
	return &ast.Identifier{Span: ast.Span{Start: t.Start, End: t.End}}
}

func (p *parser) memberName() *ast.Identifier {
	if !isSemiReserved(p.peek()) {
		p.unexpected("expected a name")
		return nil
	}
	t := p.next()
	return &ast.Identifier{Span: ast.Span{Start: t.Start, End: t.End}}
}

func (p *parser) name() *ast.Name {
	k := p.peek()
	if !isName(k) {
		p.unexpected("expected a name")
		return nil
	}
	t := p.next()
	return &ast.Name{Span: ast.Span{Start: t.Start, End: t.End}, Kind: k}
}

func isIdentifier(k token.Kind) bool {
	return k == token.Identifier
}

func isName(k token.Kind) bool {
	switch k {
	case token.Identifier, token.NameQualified, token.NameFullyQualified, token.NameRelative:
		return true
	}
	return false
}

func isSemiReserved(k token.Kind) bool {
	return k == token.Identifier || k >= token.Abstract && k <= token.TraitConst
}
