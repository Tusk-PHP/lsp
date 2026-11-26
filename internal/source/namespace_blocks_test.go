package sourcectx

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
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

func TestClassNamesEndingInResolveThroughTheirNamespaceBlock(t *testing.T) {
	src := "<?php\nnamespace Dep;\nuse X\\{Dep as D1};\nuse function F\\Dep;\nclass Dep extends Base\\Dep implements \\G\\Dep {}\nnew namespace\\Dep(); DEP::class; Dep(); Dep; $a instanceof Dep;\n" +
		"namespace B;\nuse X\\Dep;\n/** Dep */\nfunction f(?Dep $d): Dep { return Dep::make() ?? Dep::$p; }\ntrait Dep {}\nuse ;\n"
	tree := parser.ParseTreeIn(src, phpparser.OnlyGrammar(token.Latest))
	names := ClassNamesEndingIn(src, ast.NewLines(src), tree, "Dep")
	var got []string
	for offset, fqn := range names {
		got = append(got, fmt.Sprintf("%d:%s", offset, fqn))
	}
	slices.Sort(got)
	var want []string
	add := func(marker string, nth int, fqn string) {
		i := -1
		for range nth {
			i += 1 + strings.Index(src[i+1:], marker)
		}
		i += strings.Index(strings.ToLower(marker), "dep")
		want = append(want, fmt.Sprintf("%d:%s", i, fqn))
	}
	add("{Dep", 1, "X\\Dep")
	add("F\\Dep", 1, "")
	add("class Dep", 1, "Dep\\Dep")
	add("Base\\Dep", 1, "Dep\\Base\\Dep")
	add("G\\Dep", 1, "G\\Dep")
	add("namespace\\Dep", 1, "Dep\\Dep")
	add("DEP::", 1, "Dep\\DEP")
	add("instanceof Dep", 1, "Dep\\Dep")
	add("X\\Dep;", 1, "X\\Dep")
	add("?Dep", 1, "X\\Dep")
	add(": Dep", 1, "X\\Dep")
	add("Dep::make", 1, "X\\Dep")
	add("Dep::$p", 1, "X\\Dep")
	add("trait Dep", 1, "B\\Dep")
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("names =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
