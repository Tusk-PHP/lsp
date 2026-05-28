package completion

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestLaravelViewCompletionHelper(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "laravel"))
	if err != nil {
		t.Fatalf("failed to resolve Laravel testdata path: %v", err)
	}
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, root, "laravel")
	p := NewProvider(idx, ca, "laravel")
	p.SetViewResolver(frameworklaravel.NewViews(root))

	source := `<?php
function demo() {
    return view('pages::auth.lo');
}
`
	line := findLine(source, "view('pages::auth.lo")
	char := strings.Index(strings.Split(source, "\n")[line], "pages::auth.lo") + len("pages::auth.lo")

	items := p.GetCompletions("file:///app/Providers/FortifyServiceProvider.php", source, protocol.Position{Line: line, Character: char})
	labels := collectLabels(items)

	if !labels["pages::auth.login"] {
		t.Fatalf("expected namespaced Laravel view completion, got %v", mapKeys(labels, 12))
	}
}

func TestLaravelViewCompletionRouteViewSecondArg(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "laravel"))
	if err != nil {
		t.Fatalf("failed to resolve Laravel testdata path: %v", err)
	}
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, root, "laravel")
	p := NewProvider(idx, ca, "laravel")
	p.SetViewResolver(frameworklaravel.NewViews(root))

	source := `<?php
use Illuminate\Support\Facades\Route;

Route::view('/', 'dash');
`
	line := findLine(source, "Route::view")
	char := strings.Index(strings.Split(source, "\n")[line], "dash") + len("dash")

	items := p.GetCompletions("file:///routes/web.php", source, protocol.Position{Line: line, Character: char})
	labels := collectLabels(items)

	if !labels["dashboard"] {
		t.Fatalf("expected Route::view second-argument completion, got %v", mapKeys(labels, 12))
	}
}
