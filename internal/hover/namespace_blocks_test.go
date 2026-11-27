package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverResolvesThroughTheNamespaceBlockAroundTheCursor(t *testing.T) {
	unbraced := "<?php\nnamespace A;\nuse X\\Dep;\nclass Foo { function f() { new Dep(1); } }\n\nnamespace B;\nclass Bar { function g() { new Dep(2); } }\n"
	braced := "<?php\nnamespace A {\nuse X\\Dep;\nclass Foo { function f() { new Dep(1); } }\n}\nnamespace B {\nclass Bar { function g() { new Dep(2); } }\n}\nnamespace {\nclass Baz { function h() { new Dep(3); } }\n}\n"
	for _, tc := range []struct {
		name, source, marker, want string
	}{
		{"unbraced import", unbraced, "Dep(1)", "Imported X"},
		{"unbraced own namespace", unbraced, "Dep(2)", "Namespace B"},
		{"braced import", braced, "Dep(1)", "Imported X"},
		{"braced own namespace", braced, "Dep(2)", "Namespace B"},
		{"global block", braced, "Dep(3)", "Global one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := symbols.NewIndex()
			idx.IndexFile("file:///x/Dep.php", "<?php\nnamespace X;\n/** Imported X. */\nclass Dep {}\n")
			idx.IndexFile("file:///b/Dep.php", "<?php\nnamespace B;\n/** Namespace B. */\nclass Dep {}\n")
			idx.IndexFile("file:///Dep.php", "<?php\n/** Global one. */\nclass Dep {}\n")
			idx.IndexFile("file:///blocks.php", tc.source)
			p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
			i := strings.Index(tc.source, tc.marker)
			pos := protocol.Position{Line: strings.Count(tc.source[:i], "\n"), Character: i - strings.LastIndex(tc.source[:i], "\n")}
			h := p.GetHover("file:///blocks.php", tc.source, pos)
			if h == nil || !strings.Contains(h.Contents.Value, tc.want) {
				t.Errorf("hover on Dep = %+v, want the class documented %q", h, tc.want)
			}
		})
	}
}
