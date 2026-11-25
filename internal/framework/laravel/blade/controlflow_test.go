package blade

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type jumpAt struct {
	keyword  string
	level    string
	enclosed string
}

func controlFlow(t *testing.T, name, src string) []jumpAt {
	t.Helper()
	tree := phpparser.Parse(src, token.PHP83)
	if len(tree.Errors) > 0 {
		t.Fatalf("%s: %v", name, tree.Errors)
	}
	var jumps []jumpAt
	var walk func(n ast.Node, enclosing []string)
	walk = func(n ast.Node, enclosing []string) {
		switch n := n.(type) {
		case *ast.While, *ast.DoWhile, *ast.For, *ast.Foreach, *ast.Switch:
			enclosing = append(enclosing[:len(enclosing):len(enclosing)], fmt.Sprintf("%T", n))
		case *ast.FunctionDecl, *ast.Closure, *ast.ArrowFunction, *ast.ClassDecl:
			enclosing = nil
		case *ast.Break:
			jumps = append(jumps, jumpAt{"break", levelText(src, n.Levels), strings.Join(enclosing, ">")})
		case *ast.Continue:
			jumps = append(jumps, jumpAt{"continue", levelText(src, n.Levels), strings.Join(enclosing, ">")})
		}
		ast.Children(n, func(c ast.Node) { walk(c, enclosing) })
	}
	walk(tree, nil)
	return jumps
}

func levelText(src string, levels ast.Expr) string {
	if levels == nil {
		return ""
	}
	return levels.Bounds().Text(src)
}

func TestControlFlowMatchesLaravelsCompiledViews(t *testing.T) {
	templates, err := filepath.Glob("testdata/controlflow/*.blade.php")
	if err != nil || len(templates) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, path := range templates {
		name := strings.TrimSuffix(filepath.Base(path), ".blade.php")
		t.Run(name, func(t *testing.T) {
			template, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			laravel, err := os.ReadFile(strings.TrimSuffix(path, ".blade.php") + ".compiled.php")
			if err != nil {
				t.Fatal(err)
			}
			doc := Compile(string(template))
			if !doc.LaravelControlFlow {
				t.Fatal("the view's control flow isn't marked as Laravel's")
			}
			got, want := controlFlow(t, "ours", doc.PHP), controlFlow(t, "Laravel's", string(laravel))
			if !slices.Equal(got, want) {
				t.Errorf("breaks and continues, with the loops and switches around them: got %+v, Laravel compiles %+v\n%s", got, want, doc.PHP)
			}
		})
	}
}

func TestJumpsKeepTheDirectivesKeyword(t *testing.T) {
	template := "@foreach ($a as $b)\n@break\n@continue(2)\n@BREAK( -3 )\n@break($b > 1)\n@endforeach\n"
	doc := Compile(template)
	for _, want := range []string{"break;", "continue 2;", "BREAK 1;", "if ($b > 1) break;"} {
		if !strings.Contains(doc.PHP, want) {
			t.Errorf("the PHP has no %q:\n%s", want, doc.PHP)
		}
	}
	at, ok := doc.TemplateOffset(strings.Index(doc.PHP, "continue 2;"))
	if want := strings.Index(template, "continue(2)"); !ok || at != want {
		t.Errorf("continue 2 maps to %d (%v), want the directive's keyword at %d", at, ok, want)
	}
}

func TestARegisteredDirectiveHidesTheControlFlow(t *testing.T) {
	custom := &Custom{Directives: map[string]bool{"loop": true}, Conditionals: map[string]bool{"admin": true}}
	if doc := CompileWith("@admin\n@break\n@endadmin\n", CompileOptions{Custom: custom}); !doc.LaravelControlFlow {
		t.Error("a registered conditional is an if, as in Laravel's compiled view")
	}
	if doc := CompileWith("@loop($a)\n@break\n", CompileOptions{Custom: custom}); doc.LaravelControlFlow {
		t.Error("a registered directive, whose PHP is unknown, kept the control flow marked as Laravel's")
	}
	if doc := Compile("@foreach ($a as $b)\n@break\n"); doc.LaravelControlFlow {
		t.Error("an unbalanced view kept the control flow marked as Laravel's")
	}
}
