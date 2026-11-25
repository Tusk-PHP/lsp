package workspace

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestRefreshAfterComposerInstallReindexesTheVendorFiles(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("composer.json", `{"name": "app/app"}`)
	write("vendor/composer/installed.json", `{"packages": [{"name": "acme/lib", "install-path": "../acme/lib", "autoload": {"psr-4": {"Acme\\": "src/"}}}]}`)
	write("vendor/acme/lib/src/Widget.php", "<?php\nnamespace Acme;\nclass Widget {}\n")
	ws := New(Options{RootPath: root, Framework: "none", Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	if got := ws.IndexComposerDependencies(); got != 1 {
		t.Fatalf("indexed %d vendor files, want 1", got)
	}

	write("vendor/composer/installed.json", `{"packages": [{"name": "acme/extra", "install-path": "../acme/extra", "autoload": {"psr-4": {"Extra\\": "src/"}}}]}`)
	write("vendor/acme/extra/src/Thing.php", "<?php\nnamespace Extra;\nclass Thing {}\n")
	if out := refreshPaths(t, ws, root, "vendor/composer/installed.json"); !out.Vendor || out.VendorFiles != 1 {
		t.Errorf("re-indexed %d vendor files, want the new package's 1: %+v", out.VendorFiles, out)
	}
	if ws.Index.Lookup(`Extra\Thing`) == nil {
		t.Error("a package Composer added is indexed")
	}
	if ws.Index.Lookup(`Acme\Widget`) != nil {
		t.Error("a package Composer removed leaves the index")
	}
}
