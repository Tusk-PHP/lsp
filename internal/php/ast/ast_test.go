package ast_test

import (
	"go/ast"
	goparser "go/parser"
	gotoken "go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	phpast "github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func nodeTypes(t *testing.T) []string {
	t.Helper()
	file, err := goparser.ParseFile(gotoken.NewFileSet(), "ast.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, f := range st.Fields.List {
				if id, ok := f.Type.(*ast.Ident); ok && len(f.Names) == 0 && id.Name == "Span" && ts.Name.Name != "Error" {
					names = append(names, ts.Name.Name)
				}
			}
		}
	}
	sort.Strings(names)
	return names
}

func TestWalkReachesEveryNodeType(t *testing.T) {
	src, err := os.ReadFile("testdata/syntax.php")
	if err != nil {
		t.Fatal(err)
	}
	good := parser.Parse(string(src), token.Latest)
	if len(good.Errors) > 0 {
		t.Fatalf("the fixture must parse cleanly: %v", good.Errors)
	}
	bad := parser.Parse("<?php class C { 1 } $x = ; }", token.Latest)
	seen := map[string]bool{}
	for _, file := range []*phpast.File{good, bad} {
		phpast.Walk(file, func(n phpast.Node) bool {
			seen[reflect.TypeOf(n).Elem().Name()] = true
			return true
		})
	}
	for _, name := range nodeTypes(t) {
		if !seen[name] {
			t.Errorf("no node of type %s in the fixtures", name)
		}
	}
	visited := 0
	phpast.Walk(good, func(phpast.Node) bool { visited++; return false })
	if visited != 1 {
		t.Errorf("returning false stops the walk below a node: visited %d", visited)
	}
	if text := good.Stmts[0].Bounds().Text(string(src)); text != "<p>inline</p>\n" {
		t.Errorf("a span's text is its source: %q", text)
	}
}

func TestDocBeforeFindsTheCommentOfTheNextToken(t *testing.T) {
	src := "<?php\n/** first */\n/** second */\n$a = 1;\n$b = 2;\n/** trailing */"
	file := parser.Parse(src, token.Latest)
	text := func(off int) string {
		span, ok := file.DocBefore(uint32(off))
		if !ok {
			return ""
		}
		return src[span.Start:span.End]
	}
	for _, c := range []struct {
		at   string
		want string
	}{
		{"$a", "/** second */"},
		{"$b", ""},
		{"<?php", ""},
	} {
		if got := text(strings.Index(src, c.at)); got != c.want {
			t.Errorf("DocBefore(%s) = %q, want %q", c.at, got, c.want)
		}
	}
	if got := text(len(src)); got != "/** trailing */" {
		t.Errorf("DocBefore(end) = %q, want the trailing doc comment", got)
	}
}
