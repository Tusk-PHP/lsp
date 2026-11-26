package workspace

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
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

func TestIndexComposerDependenciesSkipsAutoloadFilesThatAreMissing(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"composer.json":                  `{"name": "app/app"}`,
		"vendor/composer/installed.json": `{"packages": [{"name": "acme/lib", "install-path": "../acme/lib", "autoload": {"files": ["helpers.php", "missing.php"]}}]}`,
		"vendor/acme/lib/helpers.php":    "<?php\nfunction acme_helper() {}\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ws := New(Options{RootPath: root, Framework: "none", Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	if got := ws.IndexComposerDependencies(); got != 1 {
		t.Errorf("indexed %d vendor files, want only the helper file that exists", got)
	}
}

func TestAComposerReindexLeavesFilesTheEditorOwns(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{
		"vendor/composer/installed.json": `{"packages": [{"name": "acme/lib", "install-path": "../acme/lib", "autoload": {"psr-4": {"Acme\\": "src/"}, "files": ["helpers.php"]}}]}`,
		"vendor/acme/lib/src/Widget.php": "<?php\nnamespace Acme;\nclass Widget {}\n",
		"vendor/acme/lib/helpers.php":    "<?php\nfunction acme_helper() {}\n",
	})
	widget := filepath.Join(root, "vendor/acme/lib/src/Widget.php")
	helpers := filepath.Join(root, "vendor/acme/lib/helpers.php")
	ws.IndexFile(symbols.PathToURI(widget), "<?php\nnamespace Acme;\nclass Widget {}\nclass EditedWidget {}\n")
	ws.IndexFile(symbols.PathToURI(helpers), "<?php\nfunction acme_helper() {}\nfunction edited_helper() {}\n")

	out, err := ws.Refresh(Refresh{
		Paths:      []string{filepath.Join(root, "vendor/composer/installed.json")},
		Config:     func() (*config.Config, error) { return config.LoadFromFile(ws.ConfigPath()) },
		EditorOwns: func(path string) bool { return path == widget || path == helpers },
	})
	if err != nil || !out.Vendor {
		t.Fatalf("the Composer change re-indexes the vendor files: %+v, %v", out, err)
	}
	if ws.Index.Lookup(`Acme\EditedWidget`) == nil || ws.Index.Lookup("edited_helper") == nil {
		t.Error("the editor's text of the files it has open stays indexed")
	}
}
