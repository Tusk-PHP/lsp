package workspace

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func testdataPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	all := append([]string{base}, parts...)
	return filepath.Join(all...)
}

func TestBuildIndexesProjectAndVendorSymbols(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DatabaseEnabled = boolPtr(false)

	ws, err := Build(context.Background(), Options{
		RootPath:       testdataPath(t, "project"),
		Framework:      "",
		Config:         cfg,
		BuiltinProfile: symbols.DefaultBuiltinProfile(),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if !ws.Index.Ready() {
		t.Fatal("expected index to be marked ready")
	}
	if ws.Index.Lookup(`App\User`) == nil {
		t.Fatal(`expected project symbol App\User to be indexed`)
	}
	if ws.Index.Lookup(`Monolog\Logger`) == nil {
		t.Fatal(`expected vendor symbol Monolog\Logger to be indexed`)
	}
}

func TestBuildIndexesLaravelRoutes(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DatabaseEnabled = boolPtr(false)

	ws, err := Build(context.Background(), Options{
		RootPath:       testdataPath(t, "laravel"),
		Framework:      "laravel",
		Config:         cfg,
		BuiltinProfile: symbols.DefaultBuiltinProfile(),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if ws.RouteIndex == nil {
		t.Fatal("expected Laravel route index")
	}
	names := ws.RouteIndex.Names()
	if !slices.Contains(names, "home") || !slices.Contains(names, "dashboard") {
		t.Fatalf("expected home and dashboard routes, got %v", names)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
