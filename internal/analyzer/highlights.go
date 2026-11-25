package analyzer

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func (a *Analyzer) GetDocumentHighlights(uri, source string, pos protocol.Position) []protocol.DocumentHighlight {
	doc := sourcectx.NewDocument(uri, source)
	ctx := doc.NameAt(pos)
	if ctx == nil || ctx.SymbolText == "" {
		return nil
	}
	if a.mayBeLocalVariable(ctx) {
		if highlights := highlightVariableOccurrences(source, pos, ctx.SymbolText); highlights != nil {
			return highlights
		}
	}
	target := a.resolveSymbolAt(ctx, source)
	if target == nil {
		return nil
	}
	decl := declarationIn(target, uri)
	ranges := a.occurrencesInDocument(target, uri, doc, source)
	highlights := make([]protocol.DocumentHighlight, 0, len(ranges))
	for _, rng := range ranges {
		highlights = append(highlights, protocol.DocumentHighlight{Range: rng, Kind: highlightKind(rng, decl, pos)})
	}
	return highlights
}

func declarationIn(target *symbols.Symbol, uri string) protocol.Range {
	if target.URI != uri {
		return protocol.Range{}
	}
	return target.Range
}

func (a *Analyzer) occurrencesIn(target *symbols.Symbol, uri, source string) []protocol.Range {
	if !mentions(parser.Tokenize(source), target) {
		return nil
	}
	return a.occurrencesInDocument(target, uri, sourcectx.NewDocument(uri, source), source)
}

func mentions(tokens []parser.Token, target *symbols.Symbol) bool {
	for i := range tokens {
		if mayName(tokens, i, target) {
			return true
		}
	}
	return false
}

func (a *Analyzer) occurrencesInDocument(target *symbols.Symbol, uri string, doc *sourcectx.Document, source string) []protocol.Range {
	decl := declarationIn(target, uri)
	tokens := doc.Tokens()
	var ranges []protocol.Range
	for i, tok := range tokens {
		if !mayName(tokens, i, target) {
			continue
		}
		rng := tokenRange(tok)
		if tok.Kind == parser.TokenVariable && !follows(tokens, i, parser.TokenDoubleColon) {
			if !sameRange(rng, decl) {
				continue
			}
		} else if !sameSymbol(a.resolveSymbolAt(doc.NameAt(rng.Start), source), target) {
			continue
		}
		ranges = append(ranges, rng)
	}
	return ranges
}

func (a *Analyzer) mayBeLocalVariable(ctx *sourcectx.NameContext) bool {
	return ctx.SymbolKind == sourcectx.ContextSymbolVariable && ctx.SymbolText != "$this" && ctx.AccessKind != sourcectx.AccessStatic &&
		a.index.MemberDeclaredAt(ctx.URI, ctx.Position, ctx.SymbolText) == nil
}

func mayName(tokens []parser.Token, i int, target *symbols.Symbol) bool {
	tok := tokens[i]
	name := strings.TrimPrefix(target.Name, "$")
	switch tok.Kind {
	case parser.TokenVariable:
		return target.Kind == symbols.KindProperty && tok.Value[1:] == name
	case parser.TokenIdentifier:
		if namedArgumentLabel(tokens, i) {
			return false
		}
		switch target.Kind {
		case symbols.KindFunction, symbols.KindMethod, symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
			return strings.EqualFold(tok.Value, name)
		}
		return tok.Value == name
	}
	return false
}

func namedArgumentLabel(tokens []parser.Token, i int) bool {
	next := nextSignificantToken(tokens, i+1)
	if next < 0 || tokens[next].Kind != parser.TokenColon {
		return false
	}
	prev := prevSignificantToken(tokens, i-1)
	return prev >= 0 && (tokens[prev].Kind == parser.TokenOpenParen || tokens[prev].Kind == parser.TokenComma)
}

func follows(tokens []parser.Token, i int, kind parser.TokenKind) bool {
	prev := prevSignificantToken(tokens, i-1)
	return prev >= 0 && tokens[prev].Kind == kind
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
