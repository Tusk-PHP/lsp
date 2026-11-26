package symboltest

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func PinToPHP74(t testing.TB, idx *symbols.Index, uri, source string) {
	t.Helper()
	if grammars := idx.Grammars(); !grammars.Admits(token.PHP74) || !grammars.Admits(token.Latest) {
		t.Fatalf("the index reads PHP %s, want a range from PHP 7.4 to the newest grammar", grammars)
	}
	idx.IndexFile(uri, source+"match('pin');\n")
	idx.IndexFile(uri, source)
	if tree := idx.GrammarOf(uri).Parse(source); tree.Version != token.PHP74 || !tree.Clean() {
		t.Fatalf("the index reads %s as PHP %s (clean %v), want a clean PHP 7.4 tree", uri, tree.Version, tree.Clean())
	}
	if tree := idx.Grammars().Parse(source); tree.Version != token.Latest || !tree.Clean() {
		t.Fatalf("the whole range reads %s as PHP %s (clean %v), want a clean tree of the newest grammar", uri, tree.Version, tree.Clean())
	}
}
