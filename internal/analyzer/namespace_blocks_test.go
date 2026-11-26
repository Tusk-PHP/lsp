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

func markerPosition(t *testing.T, source, marker string) protocol.Position {
	t.Helper()
	i := strings.Index(source, marker)
	if i < 0 {
		t.Fatalf("marker %q not in source", marker)
	}
	i += len(marker)
	return protocol.Position{Line: strings.Count(source[:i], "\n"), Character: i - strings.LastIndex(source[:i], "\n") - 1}
}

func TestSignatureHelpResolvesTheFunctionThroughTheNamespaceBlockAroundTheCursor(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nnamespace A;\nfunction foo(int $a) {}\nfunction bar(int $ab) {}\n")
	idx.IndexFile("file:///b.php", "<?php\nnamespace B;\nfunction foo(string $b) {}\n")
	idx.IndexFile("file:///global.php", "<?php\nfunction foo(array $g) {}\nfunction bar(float $x) {}\n")
	a := NewAnalyzer(idx, nil)
	unbraced := "<?php\nnamespace A;\nfoo(1);\nbar(1);\n\nnamespace B;\nuse function A\\bar as abar;\nuse A;\nfoo(2);\nbar(2);\nabar(2);\n\\foo(2);\nA\\foo(2);\n"
	braced := "<?php\nnamespace A {\nfoo(1);\n}\nnamespace B {\nfoo(2);\nbar(2);\n}\nnamespace {\nfoo(3);\n}\n"
	for _, tc := range []struct {
		name, source, marker, param string
	}{
		{"unbraced, own namespace", unbraced, "foo(1", "int $a"},
		{"unbraced, own namespace before global", unbraced, "bar(1", "int $ab"},
		{"unbraced, second block", unbraced, "foo(2", "string $b"},
		{"unbraced, global when the namespace has none", unbraced, "\nbar(2", "float $x"},
		{"unbraced, imported function", unbraced, "abar(2", "int $ab"},
		{"unbraced, fully qualified", unbraced, "\\foo(2", "array $g"},
		{"unbraced, qualified through an imported namespace", unbraced, "A\\foo(2", "int $a"},
		{"braced, first block", braced, "foo(1", "int $a"},
		{"braced, second block", braced, "foo(2", "string $b"},
		{"braced, global when the namespace has none", braced, "bar(2", "float $x"},
		{"global block", braced, "foo(3", "array $g"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			help := a.GetSignatureHelp("file:///calls.php", tc.source, markerPosition(t, tc.source, tc.marker))
			if help == nil || len(help.Signatures) != 1 || len(help.Signatures[0].Parameters) != 1 {
				t.Fatalf("signature help = %+v, want one signature with one parameter", help)
			}
			if got := help.Signatures[0].Parameters[0].Label; got != tc.param {
				t.Errorf("parameter = %q, want %q", got, tc.param)
			}
		})
	}
}

func TestSignatureHelpHasNoSignatureForAFunctionPHPCannotCall(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///functions.php", "<?php\nnamespace A;\nfunction only(int $a) {}\n")
	src := "<?php\nnamespace B;\nonly(1);\n"
	if help := NewAnalyzer(idx, nil).GetSignatureHelp("file:///calls.php", src, markerPosition(t, src, "only(")); help != nil {
		t.Errorf("signature help = %+v, want none: B has no only() and there is no global one", help)
	}
}

func TestImportClassActionUsesTheNamespaceBlockOfTheDiagnostic(t *testing.T) {
	unbraced := "<?php\nnamespace A;\nuse Monolog\\Logger;\nnew Logger();\n\nnamespace B;\n\nnew Logger();\n"
	braced := "<?php\nnamespace A {\nuse Monolog\\Logger;\nnew Logger();\n}\nnamespace B {\nnew Logger();\n}\nnamespace {\nnew Logger();\n}\n"
	for _, tc := range []struct {
		name, source string
		diagLine     int
		want         string
		line         int
	}{
		{"unbraced, imported in the block", unbraced, 3, "", 0},
		{"unbraced, other block", unbraced, 7, "use Monolog\\Logger;\n", 6},
		{"braced, imported in the block", braced, 3, "", 0},
		{"braced, other block", braced, 6, "use Monolog\\Logger;\n\n", 6},
		{"global block", braced, 9, "use Monolog\\Logger;\n\n", 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := setupRenameAnalyzer(map[string]string{
				"file:///test.php":          tc.source,
				"file:///vendor/logger.php": "<?php\nnamespace Monolog;\nclass Logger {}\n",
			})
			diag := unknownClassDiagnostic(tc.diagLine, 4, 10, "Unknown class 'Logger'")
			actions := a.GetCodeActions("file:///test.php", tc.source, protocol.CodeActionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
				Context:      protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{diag}, Only: []protocol.CodeActionKind{protocol.CodeActionQuickFix}},
			})
			var imports []protocol.TextEdit
			for _, action := range actions {
				if strings.HasPrefix(action.Title, "Import ") {
					imports = append(imports, action.Edit.Changes["file:///test.php"]...)
				}
			}
			if tc.want == "" {
				if len(imports) != 0 {
					t.Errorf("offered %+v, but Logger is already imported in this block", imports)
				}
				return
			}
			if len(imports) != 1 || imports[0].NewText != tc.want || imports[0].Range.Start.Line != tc.line {
				t.Errorf("imports = %+v, want %q at line %d", imports, tc.want, tc.line)
			}
		})
	}
}

func TestSignatureHelpOfAMemberCallLooksTheMemberUp(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///service.php", "<?php\nnamespace App;\nclass Service { public function handle(string $name) {} }\n")
	src := "<?php\nnamespace App;\nfunction run(Service $svc) {\n$svc->handle("
	help := NewAnalyzer(idx, nil).GetSignatureHelp("file:///call.php", src, markerPosition(t, src, "handle("))
	if help == nil || len(help.Signatures[0].Parameters) != 1 || help.Signatures[0].Parameters[0].Label != "string $name" {
		t.Errorf("signature help = %+v, want handle(string $name)", help)
	}
}
