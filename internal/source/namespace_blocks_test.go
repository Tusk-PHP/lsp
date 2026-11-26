package sourcectx

import (
	"strings"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func positionOf(t *testing.T, source, marker string) protocol.Position {
	t.Helper()
	i := strings.Index(source, marker)
	if i < 0 {
		t.Fatalf("marker %q not in source", marker)
	}
	return protocol.Position{Line: strings.Count(source[:i], "\n"), Character: i - strings.LastIndex(source[:i], "\n") - 1}
}

func TestAnalyzeTakesTheNamespaceAndImportsOfTheBlockAroundTheCursor(t *testing.T) {
	unbraced := "<?php\nnamespace A;\nuse X\\Dep;\nclass Foo { function f() { new Dep(1); } }\n\nnamespace B;\nuse Y\\Other;\nclass Bar { function g() { new Dep(2); } }\n"
	braced := "<?php\nnamespace A {\nuse X\\Dep;\nclass Foo { function f() { new Dep(1); } }\n}\n\nnamespace B {\nuse Y\\Other;\nclass Bar { function g() { new Dep(2); } }\n}\nnamespace {\nuse Z\\Glob;\nclass Baz { function h() { new Dep(3); } }\n}\n"
	for _, tc := range []struct {
		name, source, marker, namespace, uses string
	}{
		{"unbraced first", unbraced, "Dep(1)", "A", "X\\Dep"},
		{"unbraced second", unbraced, "Dep(2)", "B", "Y\\Other"},
		{"braced first", braced, "Dep(1)", "A", "X\\Dep"},
		{"braced second", braced, "Dep(2)", "B", "Y\\Other"},
		{"global block", braced, "Dep(3)", "", "Z\\Glob"},
		{"between braced blocks", braced, "\nnamespace B", "", ""},
		{"no namespace", "<?php\nuse X\\Dep;\nnew Dep(1);\n", "Dep(1)", "", "X\\Dep"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pos := positionOf(t, tc.source, tc.marker)
			ctx := Analyze("file:///blocks.php", tc.source, pos, phpparser.OnlyGrammar(token.Latest))
			if ctx.File.Namespace != tc.namespace {
				t.Errorf("Namespace = %q, want %q", ctx.File.Namespace, tc.namespace)
			}
			names := make([]string, 0, len(ctx.File.Uses))
			for _, u := range ctx.File.Uses {
				names = append(names, u.FullName)
			}
			if got := strings.Join(names, ","); got != tc.uses {
				t.Errorf("Uses = %q, want %q", got, tc.uses)
			}
		})
	}
}
