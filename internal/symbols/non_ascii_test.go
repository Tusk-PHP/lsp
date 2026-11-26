package symbols

import "testing"

const nonASCIISource = "<?php\nnamespace App;\nclass Ätna { public function größe(): int { return 1; } }\nfunction café() {}\n"

func TestNonASCIINamesAreIndexedWhole(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", nonASCIISource)
	for fqn, kind := range map[string]SymbolKind{`App\Ätna`: KindClass, `App\Ätna::größe`: KindMethod, `App\café`: KindFunction} {
		if sym := idx.Lookup(fqn); sym == nil || sym.Kind != kind {
			t.Errorf("%s is indexed as a %v, got %+v", fqn, kind, sym)
		}
	}
	if syms := idx.LookupByName("gr\xc3"); len(syms) != 0 {
		t.Errorf("no symbol is named after a cut name: %+v", syms)
	}
}
