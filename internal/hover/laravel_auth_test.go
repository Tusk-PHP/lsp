package hover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func setupLaravelAuthHover(t *testing.T) *Provider {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "laravel"))
	if err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for _, rel := range []string{
		"Support/Facades/Facade.php", "Support/Facades/Gate.php", "Support/Facades/Route.php",
		"Routing/Route.php", "Routing/RouteRegistrar.php",
	} {
		path := filepath.Join(root, "vendor", "laravel", "framework", "src", "Illuminate", filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("testdata/laravel vendor not installed: %v", err)
		}
		idx.IndexFileWithSource(symbols.PathToURI(path), string(data), symbols.SourceVendor)
	}
	idx.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Policies", "PostPolicy.php")), "<?php\nnamespace App\\Policies;\n\nclass PostPolicy\n{\n    public function publish($user, $post) {}\n}\n")
	auth := frameworklaravel.NewAuthorization(root, idx, func() phpparser.GrammarRange { return phpparser.OnlyGrammar(token.Latest) })
	auth.ScanWorkspace(composer.GetAutoloadPaths(root), nil)
	auth.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Providers", "AuthServiceProvider.php")), `<?php
namespace App\Providers;

use Illuminate\Support\Facades\Gate;

class AuthServiceProvider
{
    public function boot(): void
    {
        Gate::define('publish', fn ($user) => true);
    }
}
`)
	ca := container.NewContainerAnalyzer(idx, root, "laravel")
	ca.Analyze()
	p := NewProvider(idx, ca, "laravel")
	p.SetLaravelAuthorization(auth)
	return p
}

func hoverText(t *testing.T, p *Provider, src, needle string) string {
	t.Helper()
	at := strings.Index(src, needle) + 1
	before := src[:at]
	h := p.GetHover("file:///test.php", src, protocol.Position{Line: strings.Count(before, "\n"), Character: at - strings.LastIndex(before, "\n") - 1})
	if h == nil {
		return ""
	}
	return h.Contents.Value
}

func TestHoverLaravelAbility(t *testing.T) {
	p := setupLaravelAuthHover(t)
	text := hoverText(t, p, "<?php\nuse Illuminate\\Support\\Facades\\Gate;\nGate::allows('publish');\n", "publish")
	for _, want := range []string{"(ability) publish", "`Gate::define()` in `AuthServiceProvider.php`", "`App\\Policies\\PostPolicy::publish()`"} {
		if !strings.Contains(text, want) {
			t.Errorf("hover shows %q:\n%s", want, text)
		}
	}
}

func TestHoverLaravelMiddleware(t *testing.T) {
	p := setupLaravelAuthHover(t)
	src := "<?php\nuse Illuminate\\Support\\Facades\\Route;\nRoute::get('/', fn () => 1)->middleware(['web', 'throttle:60,1']);\n"
	if text := hoverText(t, p, src, "throttle"); !strings.Contains(text, "(middleware) throttle") || !strings.Contains(text, "Registered in `Middleware.php`") {
		t.Errorf("a parameterised alias hovers as its alias:\n%s", text)
	}
	if text := hoverText(t, p, src, "web"); !strings.Contains(text, "(middleware group) web") {
		t.Errorf("a group hovers as a group:\n%s", text)
	}
	if text := hoverText(t, p, src, "60,1"); text != "" {
		t.Errorf("the parameters are no name:\n%s", text)
	}
}
