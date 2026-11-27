package analyzer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/composer"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func setupLaravelAuthAnalyzer(t *testing.T) (*Analyzer, string) {
	t.Helper()
	a := setupLaravelAnalyzer(t)
	root, err := filepath.Abs(laravelTestdataPath())
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{
		"ProductPolicy.php":  "<?php\nnamespace App\\Policies;\n\nclass ProductPolicy\n{\n    public function update($user, $product) {}\n}\n",
		"CategoryPolicy.php": "<?php\nnamespace App\\Policies;\n\nclass CategoryPolicy\n{\n    public function update($user, $category) {}\n}\n",
	} {
		a.index.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Policies", name)), src)
	}
	auth := frameworklaravel.NewAuthorization(root, a.index, func() phpparser.GrammarRange { return phpparser.OnlyGrammar(token.Latest) })
	auth.ScanWorkspace(composer.GetAutoloadPaths(root))
	auth.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Providers", "AuthServiceProvider.php")), `<?php
namespace App\Providers;

use Illuminate\Support\Facades\Gate;

class AuthServiceProvider
{
    public function boot(): void
    {
        Gate::define('view-dashboard', fn ($user) => true);
    }
}
`)
	auth.IndexFile(symbols.PathToURI(filepath.Join(root, "bootstrap", "app.php")), `<?php
return Application::configure()->withMiddleware(function ($middleware) {
    $middleware->alias(['admin' => \App\Http\Middleware\EnsureAdmin::class]);
})->create();
`)
	a.SetLaravelAuthorization(auth)
	return a, root
}

func definitionAt(t *testing.T, a *Analyzer, src, needle string, offset int) *protocol.Location {
	t.Helper()
	at := strings.Index(src, needle)
	if at < 0 {
		t.Fatalf("%q not in source", needle)
	}
	at += offset
	before := src[:at]
	a.index.IndexFile("file:///test.php", src)
	return a.FindDefinition("file:///test.php", src, protocol.Position{Line: strings.Count(before, "\n"), Character: at - strings.LastIndex(before, "\n") - 1})
}

func TestDefinitionLaravelAbilities(t *testing.T) {
	a, root := setupLaravelAuthAnalyzer(t)
	provider := symbols.PathToURI(filepath.Join(root, "app", "Providers", "AuthServiceProvider.php"))
	productPolicy := symbols.PathToURI(filepath.Join(root, "app", "Policies", "ProductPolicy.php"))
	categoryPolicy := symbols.PathToURI(filepath.Join(root, "app", "Policies", "CategoryPolicy.php"))
	uses := "<?php\nuse App\\Models\\Product;\nuse App\\Models\\Category;\nuse Illuminate\\Support\\Facades\\Gate;\nuse Illuminate\\Support\\Facades\\Route;\n"
	tests := []struct {
		name, src, needle string
		uri               string
		line              int
	}{
		{"Gate::define", uses + "Gate::allows('view-dashboard');", "view-dashboard", provider, 9},
		{"can middleware", uses + "Route::get('/', fn () => 1)->middleware('can:view-dashboard');", "view-dashboard", provider, 9},
		{"policy of the model class", uses + "Gate::allows('update', Product::class);", "update", productPolicy, 5},
		{"policy of the model variable", uses + "function f(Category $c) { Gate::authorize('update', $c); }", "update", categoryPolicy, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := definitionAt(t, a, tt.src, tt.needle, 2)
			if loc == nil || loc.URI != tt.uri || loc.Range.Start.Line != tt.line {
				t.Errorf("definition = %+v, want %s line %d", loc, tt.uri, tt.line)
			}
		})
	}
}

func TestDefinitionLaravelMiddleware(t *testing.T) {
	a, root := setupLaravelAuthAnalyzer(t)
	uses := "<?php\nuse Illuminate\\Support\\Facades\\Route;\n"
	loc := definitionAt(t, a, uses+"Route::middleware(['web', 'auth:sanctum'])->group(fn () => 1);", "auth:", 1)
	if loc == nil || !strings.HasSuffix(loc.URI, "/Illuminate/Auth/Middleware/Authenticate.php") {
		t.Errorf("auth goes to the class it aliases: %+v", loc)
	}
	loc = definitionAt(t, a, uses+"Route::middleware('admin');", "admin", 1)
	if loc == nil || loc.URI != symbols.PathToURI(filepath.Join(root, "bootstrap", "app.php")) || loc.Range.Start.Line != 2 {
		t.Errorf("an alias whose class isn't indexed goes to its registration: %+v", loc)
	}
	loc = definitionAt(t, a, uses+"Route::middleware('web');", "web", 1)
	if loc == nil || !strings.HasSuffix(loc.URI, "/Foundation/Configuration/Middleware.php") {
		t.Errorf("Laravel's web group goes to where Laravel defines it: %+v", loc)
	}
}
