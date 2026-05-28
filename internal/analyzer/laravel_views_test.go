package analyzer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func laravelViewTestdataPath() string {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "laravel"))
	if err != nil {
		return filepath.Join("..", "..", "testdata", "laravel")
	}
	return root
}

func TestDefinitionLaravelViewHelper(t *testing.T) {
	root := laravelViewTestdataPath()
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, root, "laravel")
	a := NewAnalyzer(idx, ca)
	a.SetViewResolver(frameworklaravel.NewViews(root))

	source := `<?php
function demo() {
    return view('pages::auth.login');
}
`
	pos := charPosOf(t, source, "pages::auth.login", "view('pages::auth.login')")
	pos.Character += len("pages::")
	loc := a.FindDefinition("file:///app/Providers/FortifyServiceProvider.php", source, pos)
	if loc == nil {
		t.Fatal("expected Laravel view definition location")
	}
	if !strings.Contains(loc.URI, "/resources/views/pages/auth/login.blade.php") {
		t.Fatalf("expected login view URI, got %s", loc.URI)
	}
}

func TestDefinitionLaravelRouteViewSecondArg(t *testing.T) {
	root := laravelViewTestdataPath()
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, root, "laravel")
	a := NewAnalyzer(idx, ca)
	a.SetViewResolver(frameworklaravel.NewViews(root))

	source := `<?php
use Illuminate\Support\Facades\Route;

Route::view('/', 'dashboard');
`
	pos := charPosOf(t, source, "dashboard", "Route::view('/', 'dashboard')")
	pos.Character += len("dash")
	loc := a.FindDefinition("file:///routes/web.php", source, pos)
	if loc == nil {
		t.Fatal("expected Laravel Route::view definition location")
	}
	if !strings.Contains(loc.URI, "/resources/views/dashboard.blade.php") {
		t.Fatalf("expected dashboard view URI, got %s", loc.URI)
	}
}
