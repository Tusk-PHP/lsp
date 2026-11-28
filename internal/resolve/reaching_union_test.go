package resolve

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestWriteThatFeedsItselfKeepsItsClass(t *testing.T) {
	src := `<?php
namespace App;
class Node {
    public function next(): Node {}
}
class Run {
    public function go(): void {
        $x = new Node();
        while (true) {
            $x = $x->next();
        }
        $x;
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///run.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	pos := textpos.Position{Line: lineContaining(t, lines, "        $x;"), Character: 8}
	if got := r.ResolveVariableTypeTyped("$x", lines, pos, file); got.FQN != `App\Node` {
		t.Fatalf("$x = %q, want App\\Node", got.FullString())
	}
}

func TestNamedClassesReadsAUnionKeptInItsName(t *testing.T) {
	r := NewResolver(symbols.NewIndex())
	got := r.namedClasses(ResolvedType{FQN: `App\A|?App\B|Collection<int, App\C|null>|string`})
	want := []string{`App\A`, `App\B`, `Collection`}
	if !slices.Equal(got, want) {
		t.Fatalf("namedClasses = %q, want %q", got, want)
	}
}
