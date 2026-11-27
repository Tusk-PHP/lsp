package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestFunctionCallsResolveOnlyWherePHPLooks(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nnamespace A;\nclass Thing {}\nfunction make(): Thing {}\nfunction both(): Thing {}\n")
	idx.IndexFile("file:///global.php", "<?php\nclass G {}\nfunction both(): G {}\nfunction glob(): G {}\n")
	r := NewResolver(idx)
	inA := &parser.FileNode{Namespace: "A"}
	inB := &parser.FileNode{Namespace: "B"}
	imported := &parser.FileNode{Namespace: "B", Uses: []parser.UseNode{{FullName: "A\\make", Alias: "build", Kind: "function"}}}
	for _, tc := range []struct {
		name, expr string
		file       *parser.FileNode
		want       string
	}{
		{"own namespace", "make()", inA, "A\\Thing"},
		{"own namespace before global", "both()", inA, "A\\Thing"},
		{"case-insensitive", "MAKE()", inA, "A\\Thing"},
		{"global from another namespace", "glob()", inB, "G"},
		{"global when the namespace has none", "both()", inB, "G"},
		{"another namespace's function", "make()", inB, ""},
		{"qualified relative to the namespace", "A\\make()", inA, ""},
		{"fully qualified", "\\A\\make()", inB, "A\\Thing"},
		{"imported", "build()", imported, "A\\Thing"},
		{"imported name not found", "make()", imported, ""},
	} {
		if got := r.ExpressionType(tc.expr, ExprContext{File: tc.file}).FullString(); got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.expr, got, tc.want)
		}
	}
}

func TestFunctionsAStatementOrAClosureIsPassedToResolveOnlyWherePHPLooks(t *testing.T) {
	const functions = `<?php
namespace App;
class B {}
/** @phpstan-assert B $x */
function isB($x): void {}
/** @param \Closure(B): void $cb */
function each(\Closure $cb): void {}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///functions.php", functions)
	r := NewResolver(idx)
	for _, tc := range []struct {
		namespace, want string
	}{
		{"App", "App\\B"},
		{"Other", ""},
	} {
		src := "<?php\nnamespace " + tc.namespace + ";\nfunction run($y): void {\n    isB($y);\n    echo $y;\n    each(function ($b) {\n        echo $b;\n    });\n}\n"
		file := parser.ParseFile(src, token.Latest)
		lines := SplitLines(src)
		if got := r.ResolveVariableTypeTyped("$y", lines, textpos.Position{Line: 4, Character: 9}, file).String(); got != tc.want {
			t.Errorf("in %s, $y after isB($y) = %q, want %q", tc.namespace, got, tc.want)
		}
		if got := r.ResolveVariableTypeTyped("$b", lines, textpos.Position{Line: 6, Character: 13}, file).String(); got != tc.want {
			t.Errorf("in %s, the closure's $b = %q, want %q", tc.namespace, got, tc.want)
		}
	}
}
