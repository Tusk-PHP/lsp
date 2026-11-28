package symbols

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFileURIsAreBuiltAndReadOnlyInURIGo(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	self, err := filepath.Abs("uri.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != root && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || path == self {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if why := rawFileURI(n); why != "" {
				t.Errorf("%s: %s; use symbols.PathToURI or symbols.URIToPath", fset.Position(n.Pos()), why)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func rawFileURI(n ast.Node) string {
	switch n := n.(type) {
	case *ast.BinaryExpr:
		if n.Op == token.ADD && isFileScheme(n.X) {
			return `"file://" + path builds a file URI by hand`
		}
	case *ast.CallExpr:
		sel, ok := n.Fun.(*ast.SelectorExpr)
		if !ok || len(n.Args) != 2 || !isFileScheme(n.Args[1]) {
			return ""
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "strings" && (sel.Sel.Name == "TrimPrefix" || sel.Sel.Name == "CutPrefix") {
			return "strings." + sel.Sel.Name + `(uri, "file://") reads a file URI by hand`
		}
	case *ast.KeyValueExpr:
		key, ok := n.Key.(*ast.Ident)
		if ok && key.Name == "Scheme" && stringLit(n.Value) == "file" {
			return `url.URL{Scheme: "file"} builds a file URI by hand`
		}
	}
	return ""
}

func isFileScheme(e ast.Expr) bool {
	return stringLit(e) == "file://"
}

func stringLit(e ast.Expr) string {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return s
}
