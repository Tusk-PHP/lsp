package analyzer

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type referenceTarget struct {
	sym    *symbols.Symbol
	family []*symbols.Symbol
}

func (a *Analyzer) referenceTargetOf(sym *symbols.Symbol) referenceTarget {
	return referenceTarget{sym: sym, family: a.index.MemberFamily(sym)}
}

func (t referenceTarget) is(sym *symbols.Symbol) bool {
	return sym != nil && sym.Kind == t.sym.Kind && slices.ContainsFunc(t.family, func(member *symbols.Symbol) bool {
		return member.FQN == sym.FQN
	})
}

func (t referenceTarget) renamable() bool {
	return !slices.ContainsFunc(t.family, func(member *symbols.Symbol) bool {
		return symbols.IsBuiltin(member) || member.Source == symbols.SourceVendor || member.Source == symbols.SourceGenerated
	})
}

func (t referenceTarget) declarationsIn(uri string) []protocol.Range {
	var decls []protocol.Range
	for _, member := range t.family {
		if member.URI == uri {
			decls = append(decls, member.Range)
		}
	}
	return decls
}

func (t referenceTarget) declarations() []protocol.Location {
	var locs []protocol.Location
	for _, member := range t.family {
		if member.URI != "" && !symbols.IsBuiltin(member) {
			locs = append(locs, protocol.Location{URI: member.URI, Range: member.Range})
		}
	}
	return locs
}

func (a *Analyzer) occurrencesIn(target referenceTarget, uri, source string) []protocol.Range {
	if !mentions(source, target.sym) {
		return nil
	}
	return a.occurrencesInDocument(target, sourcectx.NewDocument(uri, source), source)
}

func mentions(source string, target *symbols.Symbol) bool {
	found := false
	parser.ScanTokens(source, func(tok parser.Token) bool {
		found = spelledLike(tok, target)
		if !found && tok.Kind == parser.TokenDocComment && target.Kind.IsClassLike() {
			docNamesSpelledLike(tok, target.Name, func(string, protocol.Range) { found = true })
		}
		return !found
	})
	return found
}

func docNamesSpelledLike(tok parser.Token, name string, fn func(written string, last protocol.Range)) {
	symbols.ForEachDocClassName(tok.Value, func(start, end int, written string) {
		lastStart := start + strings.LastIndexByte(written, '\\') + 1
		if !strings.EqualFold(tok.Value[lastStart:end], name) {
			return
		}
		fn(written, protocol.Range{Start: offsetIn(tok, lastStart), End: offsetIn(tok, end)})
	})
}

func offsetIn(tok parser.Token, offset int) protocol.Position {
	before := tok.Value[:offset]
	nl := strings.LastIndexByte(before, '\n')
	if nl < 0 {
		return protocol.Position{Line: tok.Line, Character: tok.Column + offset}
	}
	return protocol.Position{Line: tok.Line + strings.Count(before, "\n"), Character: offset - nl - 1}
}

func (a *Analyzer) occurrencesInDocument(target referenceTarget, doc *sourcectx.Document, source string) []protocol.Range {
	decls := target.declarationsIn(doc.URI())
	tokens := doc.Tokens()
	var ranges []protocol.Range
	for i, tok := range tokens {
		if tok.Kind == parser.TokenDocComment && target.sym.Kind.IsClassLike() {
			docNamesSpelledLike(tok, target.sym.Name, func(written string, last protocol.Range) {
				if strings.EqualFold(a.resolver.ResolveClassName(written, doc.File()), target.sym.FQN) {
					ranges = append(ranges, last)
				}
			})
			continue
		}
		if !mayName(tokens, i, target.sym) {
			continue
		}
		rng := tokenRange(tok)
		if tok.Kind == parser.TokenVariable && !follows(tokens, i, parser.TokenDoubleColon) {
			if !slices.Contains(decls, rng) {
				continue
			}
		} else if !target.is(a.resolveSymbolAt(doc.NameAt(rng.Start), source)) {
			continue
		}
		ranges = append(ranges, rng)
	}
	return ranges
}
