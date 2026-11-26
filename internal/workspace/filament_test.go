package workspace

import (
	"context"
	"os"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// requireFilamentWorkspace skips a test that needs the Filament demo app in
// testdata/filament. It is fetched, not committed, so a fresh clone has no
// copy until the fetch script has run.
func requireFilamentWorkspace(t *testing.T) string {
	t.Helper()
	root := testdataPath(t, "filament")
	if _, err := os.Stat(testdataPath(t, "filament", "vendor", ".fetch-lock")); err != nil {
		t.Skip("testdata/filament is missing: run `make filament-workspace`")
	}
	return root
}

// TestBuildIndexesFilamentWorkspace builds the full demo app (about 23k vendor
// PHP files) and checks that project code, Filament, Livewire and the panel
// providers all land in the index.
func TestBuildIndexesFilamentWorkspace(t *testing.T) {
	root := requireFilamentWorkspace(t)
	cfg := config.DefaultConfig()
	cfg.DatabaseEnabled = boolPtr(false)

	ws, err := Build(context.Background(), Options{
		RootPath:       root,
		Framework:      "laravel",
		Config:         cfg,
		BuiltinProfile: symbols.DefaultBuiltinProfile(),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, fqn := range []string{
		`App\Models\Shop\Product`,
		`App\Models\Shop\ProductCategory`,
		`App\Providers\Filament\AdminPanelProvider`,
		`Filament\Forms\Components\TextInput`,
		`Filament\Panel`,
		`Livewire\Component`,
		`Illuminate\Database\Eloquent\Model`,
	} {
		if ws.Index.Lookup(fqn) == nil {
			t.Errorf("expected %s to be indexed", fqn)
		}
	}
}
