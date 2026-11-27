package analyzer

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func (a *Analyzer) GetDocumentHighlights(uri, source string, pos protocol.Position) []protocol.DocumentHighlight {
	r := a.engine.Resolver()
	doc := sourcectx.NewDocument(uri, source, a.index.GrammarOf(uri))
	ctx := doc.NameAt(pos)
	if ctx == nil || ctx.SymbolText == "" {
		return nil
	}
	if a.mayBeLocalVariable(ctx) {
		if highlights := highlightVariableOccurrences(source, pos, ctx.SymbolText); highlights != nil {
			return highlights
		}
	}
	target := a.resolveSymbolAt(r, ctx, source)
	if target == nil {
		return nil
	}
	var decl protocol.Range
	if target.URI == uri {
		decl = target.Range
	}
	tokens := doc.Significant()
	var interpolated interpolatedStrings
	var highlights []protocol.DocumentHighlight
	for i, tok := range tokens {
		if interpolated.skip(tok.Kind) {
			continue
		}
		name, ok := mayName(tokens, i, source, target)
		if !ok {
			continue
		}
		rng := doc.Range(name.Start, name.End)
		if tok.Kind == token.Variable && !follows(tokens, i, token.DoubleColon) {
			if !sameRange(rng, decl) {
				continue
			}
		} else if !sameSymbol(a.resolveSymbolAt(r, doc.NameAt(rng.Start), source), target) {
			continue
		}
		highlights = append(highlights, protocol.DocumentHighlight{Range: rng, Kind: highlightKind(rng, decl, pos)})
	}
	return highlights
}

func (a *Analyzer) mayBeLocalVariable(ctx *sourcectx.NameContext) bool {
	return ctx.SymbolKind == sourcectx.ContextSymbolVariable && ctx.SymbolText != "$this" && ctx.AccessKind != sourcectx.AccessStatic &&
		a.index.MemberDeclaredAt(ctx.URI, ctx.Position, ctx.SymbolText) == nil
}

func mayName(tokens []lexer.Token, i int, src string, target *symbols.Symbol) (lexer.Token, bool) {
	tok := tokens[i]
	name := strings.TrimPrefix(target.Name, "$")
	switch tok.Kind {
	case token.Variable:
		return tok, target.Kind == symbols.KindProperty && src[tok.Start+1:tok.End] == name
	case token.NameQualified, token.NameFullyQualified, token.NameRelative:
		tok.Start += uint32(strings.LastIndexByte(tok.Text(src), '\\') + 1)
	case token.Identifier:
		if namedArgumentLabel(tokens, i) {
			return tok, false
		}
	default:
		return tok, false
	}
	switch target.Kind {
	case symbols.KindFunction, symbols.KindMethod, symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
		return tok, strings.EqualFold(tok.Text(src), name)
	}
	return tok, tok.Text(src) == name
}

func namedArgumentLabel(tokens []lexer.Token, i int) bool {
	return i > 0 && i+1 < len(tokens) && tokens[i+1].Kind == token.Colon && (tokens[i-1].Kind == token.LeftParen || tokens[i-1].Kind == token.Comma)
}

func follows(tokens []lexer.Token, i int, kind token.Kind) bool {
	return i > 0 && tokens[i-1].Kind == kind
}

type interpolatedStrings struct {
	open []interpolationFrame
}

type interpolationFrame struct {
	closer token.Kind
	braces int
}

func (s *interpolatedStrings) skip(kind token.Kind) bool {
	n := len(s.open)
	if n > 0 && s.open[n-1].closer != token.RightBrace {
		switch kind {
		case s.open[n-1].closer:
			s.open = s.open[:n-1]
		case token.CurlyOpen, token.DollarOpenCurlyBraces:
			s.open = append(s.open, interpolationFrame{closer: token.RightBrace})
		}
		return true
	}
	switch kind {
	case token.DoubleQuote:
		s.open = append(s.open, interpolationFrame{closer: kind})
	case token.StartHeredoc:
		s.open = append(s.open, interpolationFrame{closer: token.EndHeredoc})
	case token.LeftBrace:
		if n > 0 {
			s.open[n-1].braces++
		}
	case token.RightBrace:
		switch {
		case n == 0:
		case s.open[n-1].braces == 0:
			s.open = s.open[:n-1]
		default:
			s.open[n-1].braces--
		}
	}
	return n > 0 || len(s.open) > 0
}

func sameSymbol(sym, target *symbols.Symbol) bool {
	return sym != nil && sym.Kind == target.Kind && sym.FQN == target.FQN
}

func highlightKind(rng, decl protocol.Range, cursor protocol.Position) protocol.DocumentHighlightKind {
	switch {
	case sameRange(rng, decl):
		return protocol.DocumentHighlightKindWrite
	case rng.Start.Line == cursor.Line && rng.Start.Character <= cursor.Character && cursor.Character <= rng.End.Character:
		return protocol.DocumentHighlightKindText
	default:
		return protocol.DocumentHighlightKindRead
	}
}
