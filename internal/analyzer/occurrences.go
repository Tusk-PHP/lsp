package analyzer

import (
	"slices"

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
	if !mentions(parser.Tokenize(source), target.sym) {
		return nil
	}
	return a.occurrencesInDocument(target, sourcectx.NewDocument(uri, source), source)
}

func mentions(tokens []parser.Token, target *symbols.Symbol) bool {
	for i := range tokens {
		if mayName(tokens, i, target) {
			return true
		}
	}
	return false
}

func (a *Analyzer) occurrencesInDocument(target referenceTarget, doc *sourcectx.Document, source string) []protocol.Range {
	decls := target.declarationsIn(doc.URI())
	tokens := doc.Tokens()
	var ranges []protocol.Range
	for i, tok := range tokens {
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
