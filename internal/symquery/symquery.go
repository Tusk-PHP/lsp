// Package symquery provides FQN-based symbol lookup and reference resolution
// adapters that bridge the symbols index with the analyzer's FindAllReferences.
package symquery

import (
	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// DeclarationByFQN resolves an FQN to its declaring Symbol via the index.
// Returns (nil, false) when the FQN is not found or has no source URI.
func DeclarationByFQN(idx *symbols.Index, fqn string) (*symbols.Symbol, bool) {
	sym := idx.Lookup(fqn)
	if sym == nil || sym.URI == "" {
		return nil, false
	}
	return sym, true
}

// ReferencesByFQN resolves an FQN to its declaration via the index, then
// returns all references to it using the analyzer. Returns (nil, false) when
// the FQN is not found in the index, has no source URI, or readSource returns
// empty content for the declaring file.
//
// readSource is called to obtain file contents by URI; return ("", false)
// when a URI is inaccessible. The same callback is forwarded to the analyzer
// so that workspace-wide reference scanning uses the caller's document-aware
// source lookup.
func ReferencesByFQN(
	idx *symbols.Index,
	an *analyzer.Analyzer,
	fqn string,
	readSource func(uri string) (string, bool),
) ([]protocol.Location, bool) {
	sym, ok := DeclarationByFQN(idx, fqn)
	if !ok {
		return nil, false
	}

	source, ok := readSource(sym.URI)
	if !ok || source == "" {
		return nil, false
	}

	readDocument := func(uri string) string {
		s, _ := readSource(uri)
		return s
	}

	locs := an.FindAllReferences(sym.URI, source, sym.Range.Start, readDocument)
	return locs, true
}
