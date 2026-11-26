package analyzer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestNamespaceOperatorCallIsNotANamespaceDeclaration(t *testing.T) {
	const uri = "file:///workspace/ns.php"
	source := "<?php\nnamespace App;\nfunction foo() {}\nnamespace\\foo();\nfunction bar() {}\n"
	idx := symbols.NewIndex()
	idx.IndexFile(uri, source)
	if idx.Lookup("App\\bar") == nil {
		t.Errorf("bar, declared after namespace\\foo(), is not indexed as App\\bar")
	}
	a := NewAnalyzer(idx, nil)
	loc := a.FindDefinition(uri, source, protocol.Position{Line: 3, Character: 11})
	if loc == nil || loc.Range.Start.Line != 2 {
		t.Errorf("definition of namespace\\foo() = %+v, want App\\foo on line 2", loc)
	}
}
