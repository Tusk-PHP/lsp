package symbols

import (
	"slices"
	"testing"
)

func TestReplaceBuiltinsKeepsAPolyfilledInterfaceOutOfTheBuiltin(t *testing.T) {
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "8.3"})
	polyfill := "file:///vendor/polyfill/Stringable.php"
	idx.IndexFileWithSource(polyfill, "<?php\ninterface PolyBase {}\n/** Polyfilled. */\ninterface Stringable extends PolyBase { public function polyOnly(); }\n", SourceVendor)

	if err := idx.ReplaceBuiltins(BuiltinProfile{PHPVersion: "8.2"}); err != nil {
		t.Fatal(err)
	}
	defs := idx.Definitions("Stringable")
	if len(defs) != 2 || defs[0].URI != polyfill || defs[1].Source != SourceBuiltin {
		t.Fatalf("the polyfill declares Stringable over the builtin one: %+v", defs)
	}
	builtin := defs[1]
	if slices.ContainsFunc(builtin.Children, func(c *Symbol) bool { return c.Name == "polyOnly" }) ||
		slices.Contains(builtin.Implements, "PolyBase") || builtin.DocComment == "/** Polyfilled. */" {
		t.Errorf("the builtin Stringable takes nothing from the polyfill: %+v", builtin)
	}

	idx.RemoveFile(polyfill)
	sym := idx.Lookup("Stringable")
	if sym == nil || sym.Source != SourceBuiltin || idx.Lookup("Stringable::__toString") == nil {
		t.Fatalf("with the polyfill gone the builtin Stringable is back with its members: %+v", sym)
	}
	if idx.Lookup("Stringable::polyOnly") != nil || slices.Contains(sym.Implements, "PolyBase") ||
		idx.IsSubtypeOf("Stringable", "PolyBase") || slices.Contains(idx.Implementations("PolyBase"), "Stringable") {
		t.Errorf("nothing of the polyfill stays behind: %+v", sym)
	}
}
