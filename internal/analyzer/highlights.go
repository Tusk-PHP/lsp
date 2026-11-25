package analyzer

import (
	"slices"
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
	ref := a.referenceTargetOf(target)
	decls := ref.declarationsIn(uri)
	ranges := a.occurrencesInDocument(ref, doc, source)
	highlights := make([]protocol.DocumentHighlight, 0, len(ranges))
	for _, rng := range ranges {
		highlights = append(highlights, protocol.DocumentHighlight{Range: rng, Kind: highlightKind(rng, decls, pos)})
	}
	return highlights
}

func (a *Analyzer) mayBeLocalVariable(ctx *sourcectx.NameContext) bool {
	return ctx.SymbolKind == sourcectx.ContextSymbolVariable && ctx.SymbolText != "$this" && ctx.AccessKind != sourcectx.AccessStatic &&
		a.index.MemberDeclaredAt(ctx.URI, ctx.Position, ctx.SymbolText) == nil
}

func mayName(tokens []parser.Token, i int, target *symbols.Symbol) bool {
	return spelledLike(tokens[i], target) && (tokens[i].Kind != parser.TokenIdentifier || !namedArgumentLabel(tokens, i))
}

func spelledLike(tok parser.Token, target *symbols.Symbol) bool {
	name := strings.TrimPrefix(target.Name, "$")
	switch tok.Kind {
	case parser.TokenVariable:
		return target.Kind == symbols.KindProperty && tok.Value[1:] == name
	case parser.TokenIdentifier:
		if foldsCase(target.Kind) {
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

func highlightKind(rng protocol.Range, decls []protocol.Range, cursor protocol.Position) protocol.DocumentHighlightKind {
	switch {
	case slices.Contains(decls, rng):
		return protocol.DocumentHighlightKindWrite
	case rng.Start.Line == cursor.Line && rng.Start.Character <= cursor.Character && cursor.Character <= rng.End.Character:
		return protocol.DocumentHighlightKindText
	default:
		return protocol.DocumentHighlightKindRead
	}
}
