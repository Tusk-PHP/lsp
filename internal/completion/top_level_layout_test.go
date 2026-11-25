package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestMemberCompletionOnOneLineScript(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Shop.php", "<?php\nnamespace App;\nclass Shop { public function total(): int { return 1; } }\n")
	p := NewProvider(idx, nil, "none")
	source := "<?php use App\\Shop; $shop = new Shop(); $shop->"
	items := p.GetCompletions("file:///use.php", source, protocol.Position{Line: 0, Character: len(source)})
	for _, item := range items {
		if item.Label == "total" {
			return
		}
	}
	t.Errorf("total isn't offered after $shop-> on a one-line script: %d items", len(items))
}
