package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnOneLineScript(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Shop.php", "<?php\nnamespace App;\nclass Shop { public function total(): int { return 1; } }\n")
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	source := "<?php use App\\Shop; $shop = new Shop(); $shop->total();"
	for word, want := range map[string]string{"total": "function total(): int", "$shop": "Shop"} {
		hover := p.GetHover("file:///use.php", source, charPosOf(t, source, word, ""))
		if hover == nil || !strings.Contains(hover.Contents.Value, want) {
			t.Errorf("hover on %s: want %q, got %v", word, want, hover)
		}
	}
}
