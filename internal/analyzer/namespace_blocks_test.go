package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestDefinitionResolvesThroughTheNamespaceBlockAroundTheCursor(t *testing.T) {
	unbraced := "<?php\nnamespace A;\nuse X\\Dep;\nclass Foo { function f() { new Dep(1); } }\n\nnamespace B;\nclass Bar { function g() { new Dep(2); } }\n"
	braced := "<?php\nnamespace A {\nuse X\\Dep;\nclass Foo { function f() { new Dep(1); } }\n}\nnamespace B {\nclass Bar { function g() { new Dep(2); } }\n}\nnamespace {\nclass Baz { function h() { new Dep(3); } }\n}\n"
	for _, tc := range []struct {
		name, source, marker, wantURI string
	}{
		{"unbraced import", unbraced, "Dep(1)", "file:///x/Dep.php"},
		{"unbraced own namespace", unbraced, "Dep(2)", "file:///b/Dep.php"},
		{"braced import", braced, "Dep(1)", "file:///x/Dep.php"},
		{"braced own namespace", braced, "Dep(2)", "file:///b/Dep.php"},
		{"global block", braced, "Dep(3)", "file:///Dep.php"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const uri = "file:///blocks.php"
			idx := symbols.NewIndex()
			idx.IndexFile("file:///x/Dep.php", "<?php\nnamespace X;\nclass Dep {}\n")
			idx.IndexFile("file:///b/Dep.php", "<?php\nnamespace B;\nclass Dep {}\n")
			idx.IndexFile("file:///Dep.php", "<?php\nclass Dep {}\n")
			idx.IndexFile(uri, tc.source)
			i := strings.Index(tc.source, tc.marker)
			pos := protocol.Position{Line: strings.Count(tc.source[:i], "\n"), Character: i - strings.LastIndex(tc.source[:i], "\n")}
			loc := NewAnalyzer(idx, nil).FindDefinition(uri, tc.source, pos)
			if loc == nil || loc.URI != tc.wantURI {
				t.Errorf("definition of Dep = %+v, want %s", loc, tc.wantURI)
			}
		})
	}
}
