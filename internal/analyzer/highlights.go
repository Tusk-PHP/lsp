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
	if ctx.SymbolKind == sourcectx.ContextSymbolVariable && ctx.SymbolText != "$this" && ctx.AccessKind != sourcectx.AccessStatic {
		if highlights := highlightVariableOccurrences(source, pos, ctx.SymbolText); highlights != nil {
			return highlights
		}
	}
	target := a.resolveSymbolAt(ctx, source)
	if target == nil {
		return nil
	}
	var decl protocol.Range
	if target.URI == uri {
		decl = target.Range
	}
	tokens := doc.Tokens()
	var highlights []protocol.DocumentHighlight
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
		highlights = append(highlights, protocol.DocumentHighlight{Range: rng, Kind: highlightKind(rng, decl, pos)})
	}
	return highlights
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
