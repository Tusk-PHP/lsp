package parser

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/strict"
)

const eof = token.Invalid

type parser struct {
	src       string
	version   token.Version
	toks      []lexer.Token
	docs      []tokenDoc
	pos       int
	prevEnd   uint32
	emptyAt   uint32
	errors    []ast.Error
	warnings  []ast.Error
	truncated bool

	depth   int
	closers []int32

	stmtStack   scratch[ast.Stmt]
	exprStack   scratch[ast.Expr]
	argStack    scratch[*ast.Arg]
	itemStack   scratch[*ast.ArrayItem]
	paramStack  scratch[*ast.Param]
	memberStack scratch[ast.Member]
}

const (
	maxDepth            = 4096
	internalErrorPrefix = "internal parser error: "
)

func Parse(src string, version token.Version) *ast.File {
	return parse(src, version, lexer.Lex(src, version))
}

func ParseCode(src string, version token.Version) *ast.File {
	return parse(src, version, lexer.LexCode(src, version))
}

func parse(src string, version token.Version, all []lexer.Token) *ast.File {
	file := &ast.File{Span: ast.Span{End: uint32(len(src))}, Version: version}
	all, file.Shebang = skipShebang(src, all)
	p := &parser{src: src, version: version, toks: all[:0]}
	var doc ast.Span
	for _, tok := range all {
		switch tok.Kind {
		case token.Whitespace, token.OpenTag:
		case token.Comment:
			p.unterminated(tok)
		case token.DocComment:
			p.unterminated(tok)
			doc = ast.Span{Start: tok.Start, End: tok.End}
			file.Docs = append(file.Docs, ast.Doc{Comment: doc})
		default:
			if doc != (ast.Span{}) {
				p.docs = append(p.docs, tokenDoc{tok: len(p.toks), doc: doc})
				for i := len(file.Docs) - 1; i >= 0 && file.Docs[i].Token == 0; i-- {
					file.Docs[i].Token = tok.Start
				}
				doc = ast.Span{}
			}
			p.toks = append(p.toks, tok)
		}
	}
	for i := len(file.Docs) - 1; i >= 0 && file.Docs[i].Token == 0; i-- {
		file.Docs[i].Token = uint32(len(src))
	}
	p.guard(func() { p.file(file) })
	slices.SortStableFunc(p.errors, func(a, b ast.Error) int { return cmp.Compare(a.Start, b.Start) })
	file.Errors = p.errors
	file.Warnings = p.warnings
	return file
}

func skipShebang(src string, toks []lexer.Token) ([]lexer.Token, ast.Span) {
	if len(toks) == 0 || toks[0].Kind != token.InlineHTML || !strings.HasPrefix(src, "#!") {
		return toks, ast.Span{}
	}
	html := &toks[0]
	end := html.End
	if newline := strings.IndexByte(src[:end], '\n'); newline >= 0 {
		end = uint32(newline + 1)
	}
	if end == html.End {
		return toks[1:], ast.Span{End: end}
	}
	html.Start = end
	return toks, ast.Span{End: end}
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
			if strict.Enabled() {
				panic(r)
			}
			at := p.start()
			p.errors = append(p.errors, ast.Error{Span: ast.Span{Start: at, End: at}, Message: fmt.Sprintf("%s%v", internalErrorPrefix, r)})
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

func noStop(token.Kind) bool { return false }

func (p *parser) unterminated(comment lexer.Token) {
	text := comment.Text(p.src)
	if !strings.HasPrefix(text, "/*") || len(text) >= 4 && strings.HasSuffix(text, "*/") {
		return
	}
	start := ast.Span{Start: comment.Start, End: comment.Start + 2}
	if p.version < token.PHP80 {
		p.warnings = append(p.warnings, ast.Error{Span: start, Message: "unterminated comment"})
		return
	}
	p.unterminatedAt(start, "unterminated comment")
}

func (p *parser) unterminatedAt(span ast.Span, message string) {
	p.errorAt(span, message)
	p.truncated = true
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

type tokenDoc struct {
	tok int
	doc ast.Span
}

func (p *parser) docSince(from int) ast.Span {
	i, found := slices.BinarySearchFunc(p.docs, p.pos, func(d tokenDoc, pos int) int { return cmp.Compare(d.tok, pos) })
	if !found {
		i--
	}
	if i < 0 || p.docs[i].tok < from {
		return ast.Span{}
	}
	return p.docs[i].doc
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

func (p *parser) close(k token.Kind) bool {
	if p.expect(k) {
		return true
	}
	p.emptyAt = max(p.emptyAt, p.start())
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

func (p *parser) trailingComma(feature string) bool {
	commaStart := p.start()
	if !p.accept(token.Comma) {
		return false
	}
	if p.peek() == token.RightParen {
		p.requires(token.PHP80, commaStart, feature)
	}
	return true
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
		case token.New:
			return false
		case token.DoubleColon:
			if !p.onLaterLine() {
				return false
			}
		}
	}
	return p.startsStatement(p.peek())
}

func (p *parser) onLaterLine() bool {
	return p.pos > 0 && p.pos < len(p.toks) && strings.IndexByte(p.src[p.toks[p.pos-1].End:p.toks[p.pos].Start], '\n') >= 0
}

func (p *parser) closingParen(open int) int {
	if p.closers == nil {
		p.closers = matchingParens(p.toks)
	}
	return int(p.closers[open])
}

func matchingParens(toks []lexer.Token) []int32 {
	closers := make([]int32, len(toks))
	var open []int32
	for i, t := range toks {
		closers[i] = -1
		switch t.Kind {
		case token.LeftParen:
			open = append(open, int32(i))
		case token.RightParen:
			if n := len(open); n > 0 {
				closers[open[n-1]] = int32(i)
				open = open[:n-1]
			}
		}
	}
	return closers
}

func (p *parser) startsStatement(k token.Kind) bool {
	switch k {
	case token.If, token.ElseIf, token.Else, token.EndIf, token.While, token.EndWhile, token.Do, token.For, token.EndFor,
		token.Foreach, token.EndForeach, token.Switch, token.Case, token.Default, token.EndSwitch, token.Break, token.Continue,
		token.Return, token.Global, token.Echo, token.Unset, token.Try, token.Catch, token.Finally, token.Goto,
		token.Declare, token.EndDeclare, token.HaltCompiler, token.Use, token.Const, token.Class,
		token.Interface, token.Trait, token.Enum, token.Abstract, token.Final, token.Public, token.Protected, token.Private,
		token.Var, token.InlineHTML, token.OpenTagWithEcho:
		return true
	case token.Function:
		return p.isFunctionDecl()
	case token.Namespace:
		return p.nameTokens() == 0
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
	n := p.nameTokens()
	if n == 0 {
		p.unexpected("expected a name")
		return nil
	}
	name := p.takeName(n)
	return &name
}

func (p *parser) takeName(n int) ast.Name {
	start, kind := p.start(), p.nameKind(n)
	for range n {
		p.next()
	}
	return ast.Name{Span: p.span(start), Kind: kind}
}

func (p *parser) constFetch(start uint32) *ast.ConstFetch {
	n := &struct {
		ast.ConstFetch
		name ast.Name
	}{name: p.takeName(p.nameTokens())}
	n.ConstFetch = ast.ConstFetch{Name: &n.name, Span: p.span(start)}
	return &n.ConstFetch
}

func (p *parser) nameOf(allowed func(token.Kind) bool) *ast.Name {
	if n := p.nameTokens(); n > 0 && !allowed(p.nameKind(n)) {
		p.unexpected("expected a name")
	}
	return p.name()
}

func isDeclarableName(k token.Kind) bool {
	return k == token.Identifier || k == token.NameQualified
}

func isImportableName(k token.Kind) bool {
	return k != token.NameRelative
}

func (p *parser) nameKind(n int) token.Kind {
	if p.version < token.PHP80 {
		return separateNameKind(p.peek(), n)
	}
	return p.peek()
}

func (p *parser) nameTokens() int {
	k := p.peek()
	if p.version >= token.PHP80 {
		if isName(k) {
			return 1
		}
		return 0
	}
	n := 0
	switch {
	case k == token.Identifier:
		n = 1
	case k == token.NamespaceSeparator && p.peekAt(1) == token.Identifier:
		n = 2
	case k == token.Namespace && p.peekAt(1) == token.NamespaceSeparator && p.peekAt(2) == token.Identifier:
		n = 3
	default:
		return 0
	}
	for p.peekAt(n) == token.NamespaceSeparator && p.peekAt(n+1) == token.Identifier {
		n += 2
	}
	return n
}

func separateNameKind(first token.Kind, n int) token.Kind {
	switch {
	case first == token.NamespaceSeparator:
		return token.NameFullyQualified
	case first == token.Namespace:
		return token.NameRelative
	case n > 1:
		return token.NameQualified
	}
	return token.Identifier
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
