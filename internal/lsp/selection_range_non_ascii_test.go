package lsp

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestWordRangeCoversNonASCIINames(t *testing.T) {
	source := "<?php\necho PRIX_€ + $größe;\n"
	lines := ast.NewLines(source)
	for _, c := range []struct {
		at, start, end int
	}{{6, 5, 13}, {17, 16, 24}} {
		rng := findWordRange(source, lines, protocol.Position{Line: 1, Character: c.at})
		if rng.Start.Character != c.start || rng.End.Character != c.end {
			t.Errorf("word at %d: got %+v, want %d-%d", c.at, rng, c.start, c.end)
		}
	}
}
