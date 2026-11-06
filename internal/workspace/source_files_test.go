package workspace

import "testing"

func TestIsPHPSourceAndTemplate(t *testing.T) {
	for path, want := range map[string][2]bool{
		"file:///app/Model.php":                {true, false},
		"file:///view/templates/cart.phtml":    {true, true},
		"/abs/Upper.PHTML":                     {true, true},
		"file:///composer.json":                {false, false},
		"file:///assets/app.js":                {false, false},
		"file:///templates/notphtml.phtml.bak": {false, false},
	} {
		if got := IsPHPSource(path); got != want[0] {
			t.Errorf("IsPHPSource(%q) = %v, want %v", path, got, want[0])
		}
		if got := IsTemplate(path); got != want[1] {
			t.Errorf("IsTemplate(%q) = %v, want %v", path, got, want[1])
		}
	}
}
