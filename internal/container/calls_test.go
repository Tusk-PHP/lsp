package container

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func lastCall(t *testing.T, src string) (*parser.FileNode, ast.Expr) {
	t.Helper()
	file := parser.ParseFile(src, token.Latest)
	var call ast.Expr
	ast.Walk(file.Tree, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Call, *ast.MethodCall, *ast.StaticCall:
			if call == nil || n.Bounds().Start >= call.Bounds().Start {
				call = n.(ast.Expr)
			}
		}
		return true
	})
	if call == nil {
		t.Fatalf("no call in %q", src)
	}
	return file, call
}

func TestCallType(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Mailer.php", "<?php\nnamespace App;\nclass Mailer {}\nfunction helper() {}\n")
	idx.IndexFile("file:///hash.php", "<?php\nfunction hash($algo, $data) {}\n")
	ca := NewContainerAnalyzer(idx, t.TempDir(), "laravel")
	ca.Analyze()
	ca.AddBinding(&ServiceBinding{Abstract: "closure.bound", Concrete: "closure.bound"})
	ca.AddBinding(&ServiceBinding{Abstract: "mailer", Concrete: `App\Mailer`})
	r := NewEngine(idx, ca).Resolver()
	tests := []struct {
		code string
		ok   bool
		want string
	}{
		{code: "app('request')", ok: true, want: `Illuminate\Http\Request`},
		{code: `\app("request")`, ok: true, want: `Illuminate\Http\Request`},
		{code: "resolve('cache')", ok: true, want: `Illuminate\Cache\CacheManager`},
		{code: "$container->get('log')", ok: true, want: `Illuminate\Log\LogManager`},
		{code: "$this->app?->make('auth')", ok: true, want: `Illuminate\Auth\AuthManager`},
		{code: "app(\n    'mailer',\n    []\n)", ok: true, want: `App\Mailer`},
		{code: "use App\\Mailer;\napp(Mailer::class)", ok: true, want: `App\Mailer`},
		{code: "app('App\\Mailer')", ok: true, want: `App\Mailer`},
		{code: "config()", ok: true, want: `Illuminate\Config\Repository`},
		{code: "config('app.name')", ok: true},
		{code: "app('closure.bound')", ok: true},
		{code: "app()"},
		{code: "app('')"},
		{code: "app('nothing')"},
		{code: "app('hash')"},
		{code: "app('App\\helper')"},
		{code: "app($abstract)"},
		{code: "app(Mailer::CONSTANT)"},
		{code: "app($class::class)"},
		{code: "foo('request')"},
		{code: "$x->run('request')"},
		{code: "App::make('request')"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			src := "<?php\nnamespace App;\n" + tt.code + ";\n"
			file, call := lastCall(t, src)
			rt, ok := ca.CallType(r, src, file, call)
			if ok != tt.ok || rt.FQN != tt.want {
				t.Fatalf("got %q, %v; want %q, %v", rt.FQN, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestConfigWithoutABinding(t *testing.T) {
	idx := symbols.NewIndex()
	ca := NewContainerAnalyzer(idx, t.TempDir(), "none")
	src := "<?php\nconfig();\n"
	file, call := lastCall(t, src)
	if rt, ok := ca.CallType(resolve.NewResolver(idx), src, file, call); !ok || rt.FQN != `Illuminate\Config\Repository` {
		t.Fatalf("got %q, %v", rt.FQN, ok)
	}
}

func TestNewEngineWithoutAContainer(t *testing.T) {
	if NewEngine(symbols.NewIndex(), nil).ContainerCall != nil {
		t.Fatal("an engine without a container has a container hook")
	}
}
