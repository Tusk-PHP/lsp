package symbols

import "testing"

func TestIsDeprecatedReadsAnEmptyDocBlock(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///classes.php", "<?php\n/**\n */\ninterface Plain {}\n/**\n * @deprecated\n */\nclass Old {}\n")
	if plain := idx.Lookup("Plain"); plain == nil || plain.IsDeprecated() {
		t.Errorf("an interface with an empty doc block: %+v, want it indexed and not deprecated", plain)
	}
	if old := idx.Lookup("Old"); old == nil || !old.IsDeprecated() {
		t.Errorf("a class with @deprecated: %+v, want it deprecated", old)
	}
}
