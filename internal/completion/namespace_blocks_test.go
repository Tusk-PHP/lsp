package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestCompletionImportsAgainstTheNamespaceBlockAroundTheCursor(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///x/Dep.php", "<?php\nnamespace X;\nclass Dep {}\n", symbols.SourceProject)
	idx.IndexFileWithSource("file:///b/Dep.php", "<?php\nnamespace B;\nclass Dep {}\n", symbols.SourceProject)
	p := NewProvider(idx, nil, "")
	for _, tc := range []struct {
		name, source string
	}{
		{"unbraced", "<?php\nnamespace A;\nuse X\\Dep;\nclass Foo { function f() { new Dep(); } }\n\nnamespace B;\nclass Bar { function g() { new Dep"},
		{"braced", "<?php\nnamespace A {\nuse X\\Dep;\nclass Foo { function f() { new Dep(); } }\n}\nnamespace B {\nclass Bar { function g() { new Dep"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := importText(completeAt(t, p, tc.source, "X\\Dep")); got == "" {
				t.Errorf("X\\Dep, imported only in namespace A, adds no import in B")
			}
			own := completeAt(t, p, tc.source, "B\\Dep")
			if got := importText(own); got != "" {
				t.Errorf("B\\Dep in its own namespace adds %q", got)
			}
			if got := insertedName(own); got != "Dep($0)" {
				t.Errorf("B\\Dep inserted %q, want Dep($0)", got)
			}
		})
	}
}
