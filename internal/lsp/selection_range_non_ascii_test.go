package lsp

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestWordRangeCoversNonASCIINames(t *testing.T) {
	cases := []struct {
		at, start, end int
	}{{6, 5, 13}, {17, 16, 24}}
	positions := make([]protocol.Position, len(cases))
	for i, c := range cases {
		positions[i] = protocol.Position{Line: 1, Character: c.at}
	}
	chains := buildSelectionChains("<?php\necho PRIX_€ + $größe;\n", positions)
	for i, c := range cases {
		if want := selectionRng(1, c.start, 1, c.end); chains[i].Range != want {
			t.Errorf("word at %d: got %+v, want %+v", c.at, chains[i].Range, want)
		}
	}
}
