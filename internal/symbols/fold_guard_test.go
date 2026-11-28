package symbols

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamesFoldWithTheASCIIHelpersOnly(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "strings" && (sel.Sel.Name == "ToLower" || sel.Sel.Name == "EqualFold") {
				t.Errorf("%s: strings.%s folds Unicode case; PHP names fold ASCII only (FoldName, FoldKey, EqualFold)", fset.Position(sel.Pos()), sel.Sel.Name)
			}
			return true
		})
	}
}

func TestMayNarrowFoldsASCIICaseOnly(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nclass Guard {}\n")
	idx.AddVirtualMember("Guard", &Symbol{Name: "Ässert", FQN: "Guard::Ässert", Kind: KindMethod, URI: "file:///a.php", DocComment: "/** @phpstan-assert int $x */"})
	if !idx.MayNarrow("ÄSSERT") {
		t.Error("ÄSSERT calls Ässert, which narrows")
	}
	if idx.MayNarrow("ässert") {
		t.Error("ässert is not Ässert")
	}
}

func TestOptionalNamespacesFoldASCIICaseOnly(t *testing.T) {
	idx := NewIndex()
	idx.SetOptionalNamespaces([]string{`faker\`, `ätna\`})
	for fqn, want := range map[string]bool{`\Faker\Generator`: true, `ätna\Lava`: true, `Ätna\Lava`: false, `App\Faker`: false} {
		if got := idx.IsOptionalClass(fqn); got != want {
			t.Errorf("IsOptionalClass(%s) = %v, want %v", fqn, got, want)
		}
	}
}

func TestALaterBuiltinLayerMovesAClassToItsNewParent(t *testing.T) {
	idx := NewIndex()
	idx.IndexFileWithSource("builtin://core.php", "<?php\nclass OldBase {}\nclass NewBase {}\nclass Shim extends OldBase {}\n", SourceBuiltin)
	idx.IndexFileWithSource("builtin://delta.php", "<?php\nclass Shim extends NewBase {}\n", SourceBuiltin)
	if got := idx.Subclasses("OldBase"); len(got) != 0 {
		t.Errorf("Shim no longer extends OldBase: %v", got)
	}
	if got := idx.Subclasses("NEWBASE"); len(got) != 1 || got[0] != "Shim" {
		t.Errorf("Shim extends NewBase: %v", got)
	}
}
