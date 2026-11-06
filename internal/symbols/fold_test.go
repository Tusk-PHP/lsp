package symbols

import "testing"

func TestLookupFoldMatchesPHPCaseRules(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nclass PclZip { const OPT = 1; }\nfunction floatHelper() {}\nclass Init {}\nfunction init() {}\n")

	if sym := idx.LookupFold("pclzip", nil); sym == nil || sym.FQN != "PclZip" {
		t.Errorf("pclzip should find PclZip, got %+v", sym)
	}
	if sym := idx.LookupFold("FLOATHELPER", nil); sym == nil || sym.FQN != "floatHelper" {
		t.Errorf("FLOATHELPER should find floatHelper, got %+v", sym)
	}
	isFn := func(s *Symbol) bool { return s.Kind == KindFunction }
	isClass := func(s *Symbol) bool { return s.Kind == KindClass }
	if sym := idx.LookupFold("INIT", isFn); sym == nil || sym.FQN != "init" {
		t.Errorf("INIT as a function should find init(), got %+v", sym)
	}
	if sym := idx.LookupFold("init", isClass); sym == nil || sym.FQN != "Init" {
		t.Errorf("init as a class should find Init, got %+v", sym)
	}
	// Class constants and members stay case-sensitive.
	if sym := idx.LookupFold("PCLZIP::opt", nil); sym != nil {
		t.Errorf("constants must not be matched case-insensitively, got %+v", sym)
	}

	// Re-indexing the file without the class removes it from the fold map.
	idx.IndexFile("file:///a.php", "<?php\nfunction other() {}\n")
	if sym := idx.LookupFold("pclzip", nil); sym != nil {
		t.Errorf("removed class still found: %+v", sym)
	}
}
