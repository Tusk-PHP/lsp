// Package inlayhint implements the textDocument/inlayHint LSP request handler.
// It provides five categories of inline type and parameter-name annotations for
// PHP source files: variable type hints, foreach variable types, closure return
// types, method return types, and call-site parameter name labels.
package inlayhint

import (
	"sort"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Provider produces inlay hints for a PHP source file.
// It is designed to be constructed once and reused across requests.
type Provider struct {
	index  *symbols.Index
	engine *resolve.Engine
}

// NewProvider creates a Provider backed by the shared symbol index.
func NewProvider(index *symbols.Index) *Provider {
	return &Provider{index: index, engine: resolve.NewEngine(index)}
}

// GetInlayHints returns all inlay hints for the given source file, filtered by
// the configuration flags.  The returned slice is sorted by line then character
// for deterministic output.  Range filtering (to params.Range) is performed by
// the LSP handler, not here.
func (p *Provider) GetInlayHints(uri, source string, cfg *config.InlayHintsConfig) []protocol.InlayHint {
	if !cfg.Enabled {
		return nil
	}

	file := parser.ParseFileIn(source, p.index.GrammarOf(uri))
	r := p.engine.Resolver()
	h := &hinter{
		index:   p.index,
		r:       r,
		file:    file,
		lines:   r.Lines(source),
		offsets: ast.NewLines(source),
		cfg:     cfg,
	}
	ast.Walk(file.Tree, h.visit)

	sort.SliceStable(h.hints, func(i, j int) bool {
		a, b := h.hints[i].Position, h.hints[j].Position
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Character < b.Character
	})
	return h.hints
}
