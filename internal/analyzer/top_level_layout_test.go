package analyzer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestDefinitionOnOneLineScript(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Shop.php", "<?php\nnamespace App;\nclass Shop {\n    public function total(): int { return 1; }\n}\n")
	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))
	source := "<?php use App\\Shop; $shop = new Shop(); $shop->total();"
	loc := a.FindDefinition("file:///use.php", source, charPosOf(t, source, "total", ""))
	if loc == nil || loc.URI != "file:///Shop.php" || loc.Range.Start.Line != 3 {
		t.Errorf("definition of total on a one-line script: got %v, want Shop.php:3", loc)
	}
}
