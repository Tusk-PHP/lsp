package sourcectx

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func LexNamespaces(src string) *Namespaces {
	return namespacesOfTokens(src, lex(src))
}

func lex(src string) []lexer.Token {
	if !parser.HasOpenTag(src) {
		return lexer.LexCode(src, token.Latest)
	}
	return lexer.Lex(src, token.Latest)
}

type namespaceReader struct {
	src     string
	lines   ast.Lines
	tokens  []lexer.Token
	next    int
	last    lexer.Token
	blocks  []namespaceBlock
	cur     namespaceBlock
	braced  *namespaceBlock
	depth   int
	leading bool
}

func namespacesOfTokens(src string, tokens []lexer.Token) *Namespaces {
	r := &namespaceReader{src: src, lines: ast.NewLines(src), tokens: tokens, leading: true}
	if len(tokens) > 0 && tokens[0].Kind == token.InlineHTML {
		r.cur.header = tokens[0].End
	}
	r.read()
	last := r.cur
	if r.braced != nil {
		last = *r.braced
	}
	return &Namespaces{src: src, lines: r.lines, blocks: append(r.blocks, last)}
}

func (r *namespaceReader) peek() lexer.Token {
	for i := r.next; i < len(r.tokens); i++ {
		if !r.tokens[i].Kind.IsTrivia() {
			r.next = i
			return r.tokens[i]
		}
	}
	r.next = len(r.tokens)
	return lexer.Token{}
}

func (r *namespaceReader) take() lexer.Token {
	tok := r.peek()
	if tok.Kind != token.Invalid {
		r.next++
		r.last = tok
	}
	return tok
}

func (r *namespaceReader) accept(kind token.Kind) (lexer.Token, bool) {
	if tok := r.peek(); tok.Kind == kind {
		return r.take(), true
	}
	return lexer.Token{}, false
}

func (r *namespaceReader) read() {
	statement := true
	for {
		tok := r.take()
		switch {
		case tok.Kind == token.Invalid:
			return
		case statement && r.atStatementLevel():
			r.statement(tok)
		case tok.Kind == token.LeftBrace || tok.Kind == token.CurlyOpen || tok.Kind == token.DollarOpenCurlyBraces:
			r.depth++
		case tok.Kind == token.RightBrace:
			r.closeBrace(tok)
		}
		statement = startsStatement(r.last.Kind)
	}
}

func (r *namespaceReader) atStatementLevel() bool {
	if r.braced != nil {
		return r.depth == 1
	}
	return r.depth == 0
}

func startsStatement(kind token.Kind) bool {
	switch kind {
	case token.Semicolon, token.LeftBrace, token.RightBrace, token.OpenTag, token.CloseTag, token.InlineHTML:
		return true
	}
	return false
}

func (r *namespaceReader) closeBrace(tok lexer.Token) {
	r.depth = max(r.depth-1, 0)
	if r.braced == nil || r.depth > 0 {
		return
	}
	r.blocks = append(r.blocks, *r.braced)
	r.braced = nil
	r.cur = namespaceBlock{start: tok.End + 1, header: tok.Start}
}

func (r *namespaceReader) statement(tok lexer.Token) {
	switch tok.Kind {
	case token.Namespace:
		if r.braced == nil {
			r.namespace(tok)
		}
	case token.Use:
		r.leading = false
		r.use(tok)
	case token.Declare:
		r.declare()
	case token.LeftBrace:
		r.depth++
	case token.RightBrace:
		r.closeBrace(tok)
	case token.Semicolon, token.OpenTag, token.CloseTag, token.InlineHTML:
	default:
		r.leading = false
	}
}

func (r *namespaceReader) namespace(keyword lexer.Token) {
	r.blocks = append(r.blocks, r.cur)
	r.leading = false
	block := namespaceBlock{start: keyword.Start}
	if name, ok := r.name(token.Identifier, token.NameQualified); ok {
		block.name, block.nameAt = name.Text(r.src), ast.Span{Start: name.Start, End: name.End}
	}
	if brace, ok := r.accept(token.LeftBrace); ok {
		block.braced, block.brace = true, brace.Start
		r.depth = 1
		r.braced = &block
		return
	}
	block.header = r.endStatement() - 1
	r.cur = block
}

func (r *namespaceReader) name(kinds ...token.Kind) (lexer.Token, bool) {
	tok := r.peek()
	for _, kind := range kinds {
		if tok.Kind == kind {
			return r.take(), true
		}
	}
	return lexer.Token{}, false
}

func (r *namespaceReader) endStatement() uint32 {
	if tok := r.peek(); tok.Kind == token.Semicolon || tok.Kind == token.CloseTag {
		r.take()
	}
	return r.last.End
}

func (r *namespaceReader) block() *namespaceBlock {
	if r.braced != nil {
		return r.braced
	}
	return &r.cur
}

func (r *namespaceReader) use(keyword lexer.Token) {
	block := r.block()
	first := len(block.imports)
	kind := r.useKind(token.Use)
	prefix, ok := r.name(token.Identifier, token.NameQualified, token.NameFullyQualified)
	if ok && r.peek().Kind == token.NamespaceSeparator {
		r.take()
		r.group(block, prefix.Text(r.src)+"\\", kind)
	} else {
		if ok {
			r.item(block, "", prefix, kind)
		}
		for _, more := r.accept(token.Comma); more; _, more = r.accept(token.Comma) {
			if name, ok := r.name(token.Identifier, token.NameQualified, token.NameFullyQualified); ok {
				r.item(block, "", name, kind)
			}
		}
	}
	end := r.endStatement()
	for i := first; i < len(block.imports); i++ {
		block.imports[i].stmt = ast.Span{Start: keyword.Start, End: end}
	}
	block.lastUse = end
}

func (r *namespaceReader) useKind(kind token.Kind) token.Kind {
	if k := r.peek().Kind; k == token.Function || k == token.Const {
		r.take()
		return k
	}
	return kind
}

func (r *namespaceReader) group(block *namespaceBlock, prefix string, kind token.Kind) {
	if _, ok := r.accept(token.LeftBrace); !ok {
		return
	}
	for {
		itemKind := kind
		if kind == token.Use {
			itemKind = r.useKind(kind)
		}
		if name, ok := r.name(token.Identifier, token.NameQualified); ok {
			r.item(block, prefix, name, itemKind)
		}
		if _, more := r.accept(token.Comma); !more {
			break
		}
	}
	r.accept(token.RightBrace)
}

func (r *namespaceReader) item(block *namespaceBlock, prefix string, name lexer.Token, kind token.Kind) {
	alias := ""
	if _, ok := r.accept(token.As); ok {
		if tok, ok := r.name(token.Identifier); ok {
			alias = tok.Text(r.src)
		}
	}
	line, _ := r.lines.Position(name.Start)
	block.uses = append(block.uses, parser.NewUseNode(prefix+name.Text(r.src), alias, kind, line))
	block.imports = append(block.imports, importSite{nameEnd: name.End})
}

func (r *namespaceReader) declare() {
	if _, ok := r.accept(token.LeftParen); ok && !r.closeParens() {
		return
	}
	if tok := r.peek(); r.leading && (tok.Kind == token.Semicolon || tok.Kind == token.CloseTag) {
		r.cur.header = r.endStatement() - 1
	}
}

func (r *namespaceReader) closeParens() bool {
	for depth := 1; depth > 0; {
		switch r.take().Kind {
		case token.LeftParen:
			depth++
		case token.RightParen:
			depth--
		case token.Invalid:
			return false
		}
	}
	return true
}
