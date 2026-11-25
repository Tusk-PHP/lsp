package symbols

import "testing"

func TestBuiltinRegisteredTwiceIsListedOnceAsTheLaterSymbol(t *testing.T) {
	idx := NewIndex()
	idx.mu.Lock()
	idx.addBuiltinFunctionLocked("strlen", nil, "int", "first")
	idx.addBuiltinClassLikeLocked("Closure", "a class")
	idx.addBuiltinFunctionLocked("strlen", nil, "int", "second")
	idx.mu.Unlock()

	var listed []*Symbol
	for _, sym := range idx.fileSymbols["builtin"] {
		if sym.FQN == "strlen" {
			listed = append(listed, sym)
		}
	}
	if len(listed) != 1 || listed[0].DocComment != "second" || idx.Lookup("strlen") != listed[0] {
		t.Fatalf("the builtin file lists strlen once, as the later registration: %+v", listed)
	}
	if got := idx.LookupByName("strlen"); len(got) != 1 {
		t.Errorf("strlen is found by name once: %v", got)
	}
}

func TestRegisteredBuiltinsAreListedOncePerName(t *testing.T) {
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "8.3"})
	seen := make(map[string]bool)
	for _, sym := range idx.fileSymbols["builtin"] {
		if seen[sym.FQN] {
			t.Fatalf("%s is listed twice in the builtin file", sym.FQN)
		}
		seen[sym.FQN] = true
	}
}
