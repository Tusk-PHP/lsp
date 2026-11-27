package symbols

import "testing"

func TestDefineDeclaresAGlobalConstant(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///boot.php", `<?php
namespace App;
define('APP_VERSION', '1.2');
define('App\Config\DEBUG', true);
const LOCAL = 1;
`)
	cases := []struct{ fqn, name, value string }{
		{"APP_VERSION", "APP_VERSION", "'1.2'"},
		{`App\Config\DEBUG`, "DEBUG", "true"},
		{`App\LOCAL`, "LOCAL", "1"},
	}
	for _, c := range cases {
		sym := idx.Lookup(c.fqn)
		if sym == nil || sym.Kind != KindConstant || sym.Name != c.name || sym.Value != c.value || sym.ParentFQN != "" || sym.URI != "file:///boot.php" {
			t.Errorf("Lookup(%q) = %+v", c.fqn, sym)
		}
	}
	if sym := idx.Lookup(`App\APP_VERSION`); sym != nil {
		t.Errorf("define() ignores the file's namespace: %+v", sym)
	}
	if sym := idx.Lookup("APP_VERSION"); sym != nil && (sym.Range.Start.Line != 2 || sym.Range.Start.Character != 8) {
		t.Errorf("APP_VERSION is declared at its name in the string: %+v", sym.Range)
	}

	idx.IndexFile("file:///boot.php", "<?php\n")
	if sym := idx.Lookup("APP_VERSION"); sym != nil {
		t.Errorf("a removed define() leaves the index: %+v", sym)
	}
}
