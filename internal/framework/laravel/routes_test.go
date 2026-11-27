package laravel

import (
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouteIndexScanWorkspaceFindsNamedRoutes(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "laravel")
	idx := NewRouteIndex(root, func() phpparser.GrammarRange { return phpparser.OnlyGrammar(token.Latest) })

	if err := idx.ScanWorkspace(); err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}

	want := []string{"appearance.edit", "dashboard", "home", "profile.edit", "security.edit"}
	names := idx.Names()
	for _, name := range want {
		found := false
		for _, got := range names {
			if got == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected route %q in %v", name, names)
		}
	}

	security := idx.Find("security.edit")
	if security == nil {
		t.Fatal("expected definition for security.edit")
	}
	if !strings.Contains(security.URI, "/routes/settings.php") {
		t.Fatalf("expected settings route URI, got %s", security.URI)
	}
	if security.Range.Start.Line != 23 {
		t.Fatalf("expected security.edit on line 23, got %d", security.Range.Start.Line)
	}
}

func TestRouteIndexReplacesEntriesOnReindex(t *testing.T) {
	idx := NewRouteIndex("/tmp/project", func() phpparser.GrammarRange { return phpparser.OnlyGrammar(token.Latest) })
	uri := "file:///tmp/project/routes/web.php"

	idx.IndexFile(uri, "<?php\nRoute::get('/')->name('home');\n")
	if got := idx.Find("home"); got == nil {
		t.Fatal("expected home after initial index")
	}

	idx.IndexFile(uri, "<?php\nRoute::get('/')->name('dashboard');\n")
	if got := idx.Find("home"); got != nil {
		t.Fatal("expected home to be removed after reindex")
	}
	if got := idx.Find("dashboard"); got == nil {
		t.Fatal("expected dashboard after reindex")
	}
}

func TestRouteIndexCapturesControllerMetadata(t *testing.T) {
	idx := NewRouteIndex("/tmp/project", func() phpparser.GrammarRange { return phpparser.OnlyGrammar(token.Latest) })
	uri := "file:///tmp/project/routes/web.php"
	source := `<?php

use App\Http\Controllers\HomeController;
use Illuminate\Support\Facades\Route;

Route::get('/', [HomeController::class, 'index'])->name('home');
Route::post('/invokable', HomeController::class)->name('home.invoke');
Route::view('/welcome', 'welcome')->name('welcome');
`

	idx.IndexFile(uri, source)

	home := idx.Find("home")
	if home == nil {
		t.Fatal("expected home route")
	}
	if home.Path != "/" {
		t.Fatalf("expected route path '/', got %q", home.Path)
	}
	if home.TargetKind != "controller" {
		t.Fatalf("expected controller target, got %q", home.TargetKind)
	}
	if home.HandlerFQN != `App\Http\Controllers\HomeController` {
		t.Fatalf("expected controller FQN, got %q", home.HandlerFQN)
	}
	if home.HandlerMethod != "index" {
		t.Fatalf("expected handler method index, got %q", home.HandlerMethod)
	}

	invoke := idx.Find("home.invoke")
	if invoke == nil {
		t.Fatal("expected home.invoke route")
	}
	if invoke.HandlerMethod != "__invoke" {
		t.Fatalf("expected __invoke handler, got %q", invoke.HandlerMethod)
	}

	welcome := idx.Find("welcome")
	if welcome == nil {
		t.Fatal("expected welcome route")
	}
	if welcome.TargetKind != "view" || welcome.Target != "welcome" {
		t.Fatalf("expected view target welcome, got kind=%q target=%q", welcome.TargetKind, welcome.Target)
	}
}
