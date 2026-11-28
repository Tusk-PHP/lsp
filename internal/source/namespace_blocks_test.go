package sourcectx

import (
	"fmt"
	"reflect"
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

func TestClassNameAtResolvesThroughItsNamespaceBlock(t *testing.T) {
	src := "<?php\nnamespace Dep;\nuse X\\{Dep as D1};\nuse function F\\Dep;\nclass Dep extends Base\\Dep implements \\G\\Dep {}\nnew namespace\\Dep(); Dep\\Sub::class; DEP::class;\n" +
		"namespace B;\nuse X\\Dep, Y\\Other as Dep2;\n/** Dep */\nfunction f(?Dep $d): Dep { return 'Q\\Dep' . Dep::make(); } // C\\Dep\nuse X\\Dep\\Inner;\n"
	namespaces := LexNamespaces(src)
	type found struct {
		fqn    string
		isName bool
	}
	var got []string
	lineStart := 0
	for lineNum, line := range strings.Split(src, "\n") {
		for col := 0; col+3 <= len(line); col++ {
			if !strings.EqualFold(line[col:col+3], "dep") || col > 0 && isWordByte(line[col-1]) || col+3 < len(line) && isWordByte(line[col+3]) {
				continue
			}
			fqn, isName := namespaces.ClassNameAt(line, uint32(lineStart), col, 3)
			got = append(got, fmt.Sprintf("%d:%d %v", lineNum, col, found{fqn, isName}))
		}
		lineStart += len(line) + 1
	}
	want := []string{
		"1:10 { true}",
		"2:7 {X\\Dep true}",
		"3:15 { true}",
		"4:6 {Dep\\Dep true}",
		"4:23 {Dep\\Base\\Dep true}",
		"4:41 {G\\Dep true}",
		"5:14 {Dep\\Dep true}",
		"5:21 { true}",
		"5:37 {Dep\\DEP true}",
		"7:6 {X\\Dep true}",
		"8:4 {X\\Dep true}",
		"9:12 {X\\Dep true}",
		"9:21 {X\\Dep true}",
		"9:37 { false}",
		"9:44 {X\\Dep true}",
		"9:64 { false}",
		"10:6 { true}",
	}
	if !slices.Equal(got, want) {
		t.Errorf("class names =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func isWordByte(c byte) bool {
	return c == '_' || c == '$' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

var namespaceSources = map[string]string{
	"unbraced":               "<?php\nnamespace A;\nuse X\\Dep;\nclass Foo {}\n\nnamespace B;\nuse Y\\Other, Z\\Third as T;\nclass Bar {}\n",
	"braced":                 "<?php\nnamespace A {\nuse X\\Dep;\nclass Foo { function f() { return \"{$x}${y}\"; } }\n}\n\nnamespace B /* b */\n{\nuse Y\\Other;\n}\nnamespace {\nuse Z\\Glob;\n}\n",
	"empty braced":           "<?php\nnamespace A {}\nnamespace {}\n",
	"no namespace":           "<?php\nuse X\\Dep;\nnew Dep(1);\n",
	"declare":                "<?php\ndeclare(strict_types=1);\n\nuse X\\Dep;\n",
	"declare only":           "<?php\ndeclare(strict_types=1);\nfoo();\ndeclare(ticks=1);\n",
	"declare block":          "<?php\ndeclare(ticks=1) { use X; }\nfoo();\n",
	"declare unclosed":       "<?php\ndeclare(strict_types=(1",
	"inline html":            "<html>\n<?php\nnamespace A;\n",
	"html after code":        "<?php\n/** doc */\n?>\n<html><?php echo 1; ?>\n",
	"shebang":                "#!/usr/bin/env php\n<?php\nnamespace A;\nuse X;\n",
	"code without tag":       "namespace A;\nuse X\\Dep;\n",
	"close tag":              "<?php\nnamespace A ?>\n<?php use X\\Dep ?>\nhtml\n",
	"group use":              "<?php\nnamespace A;\nuse X\\{Dep, function f, const C as K, Sub\\Deep as D};\nuse function Y\\{g, h};\nuse \\Z\\{W};\n",
	"function and const":     "<?php\nnamespace A;\nuse function X\\f, X\\g as h;\nuse const X\\C;\n",
	"broken use":             "<?php\nnamespace A;\nuse ;\nuse X\\{;\nuse Y as ;\nuse Z,",
	"closure use":            "<?php\nnamespace A;\n$f = function () use ($x) {};\nuse X\\Dep;\n",
	"namespace operator":     "<?php\nnamespace A;\nuse X\\Dep;\nnamespace\\foo();\nfunction bar() {}\n",
	"heredoc":                "<?php\nnamespace A;\n$s = <<<EOT\n{$a} }\nEOT;\nuse X\\Dep;\n",
	"group without brace":    "<?php\nnamespace A;\nuse X\\;\nuse Y;\n",
	"nested namespace":       "<?php\nnamespace A {\nnamespace B;\nuse X;\n}\n",
	"top-level block":        "<?php\nnamespace A;\n{ use X; }\nuse Y;\n",
	"declare without parens": "<?php\ndeclare;\nuse X;\n",
	"stray brace":            "<?php\nnamespace A;\n}\nuse X\\Dep;\n",
	"unnamed unbraced":       "<?php\nnamespace;\nuse X;\n",
	"unterminated":           "<?php\nnamespace A",
}

func TestLexNamespacesReadsTheBlocksTheSyntaxTreeHas(t *testing.T) {
	for name, src := range namespaceSources {
		t.Run(name, func(t *testing.T) {
			assertSameNamespaces(t, src)
		})
	}
}

func assertSameNamespaces(t *testing.T, src string) {
	t.Helper()
	if !sameNamespaces(src) {
		tree := parser.ParseTreeIn(src, phpparser.OnlyGrammar(token.Latest))
		t.Errorf("lexer blocks\n%#v\nsyntax tree blocks\n%#v", LexNamespaces(src).blocks, NamespacesOf(src, ast.NewLines(src), tree).blocks)
	}
}

func sameNamespaces(src string) bool {
	tree := parser.ParseTreeIn(src, phpparser.OnlyGrammar(token.Latest))
	fromTree := NamespacesOf(src, ast.NewLines(src), tree)
	if !reflect.DeepEqual(LexNamespaces(src).blocks, fromTree.blocks) {
		return false
	}
	head := LexNamespaces(src[:parser.FileNodeOf(src, tree).ImportsEnd])
	for _, tok := range lex(src) {
		if tok.Kind.IsTrivia() {
			continue
		}
		if got, want := head.At(tok.Start), fromTree.At(tok.Start); got.Name != want.Name || !slices.Equal(got.Uses, want.Uses) {
			return false
		}
	}
	return true
}

func TestUseSiteInsertsTheImportWhereTheBlockAroundTheCursorKeepsThem(t *testing.T) {
	for _, tc := range []struct {
		name, source, marker string
		want                 protocol.TextEdit
	}{
		{"after the last import", "<?php\nnamespace A;\nuse X;\nclass Foo { function f() { CURSOR; } }\n", "CURSOR",
			protocol.TextEdit{Range: protocol.Range{Start: protocol.Position{Line: 3}, End: protocol.Position{Line: 3}}, NewText: "use Y;\n"}},
		{"after the header, before a blank line", "<?php\nnamespace A;\n\nclass Foo { function f() { CURSOR; } }\n", "CURSOR",
			protocol.TextEdit{Range: protocol.Range{Start: protocol.Position{Line: 2}, End: protocol.Position{Line: 2}}, NewText: "use Y;\n"}},
		{"inside a braced block, before code", "<?php\nnamespace A {\nclass Foo { function f() { CURSOR; } }\n}\nnamespace B {\nuse Z;\n}\n", "CURSOR",
			protocol.TextEdit{Range: protocol.Range{Start: protocol.Position{Line: 2}, End: protocol.Position{Line: 2}}, NewText: "use Y;\n\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := parser.ParseTreeIn(tc.source, phpparser.OnlyGrammar(token.Latest))
			namespaces := NamespacesOf(tc.source, ast.NewLines(tc.source), tree)
			site := namespaces.UseSiteAt(namespaces.Offset(positionOf(t, tc.source, tc.marker)))
			if got := site.Insert("use Y;\n"); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Insert = %+v, want %+v", got, tc.want)
			}
		})
	}
}
