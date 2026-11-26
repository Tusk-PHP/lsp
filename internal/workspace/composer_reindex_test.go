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

func TestAVendorFileChangedOnDiskIsReindexedFromDisk(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{
		"vendor/composer/installed.json": `{"packages": [{"name": "acme/lib", "install-path": "../acme/lib", "autoload": {"psr-4": {"Acme\\": "src/"}}}]}`,
		"vendor/acme/lib/src/Widget.php": "<?php\nnamespace Acme;\nclass Widget {}\n",
		"vendor/acme/lib/src/Gone.php":   "<?php\nnamespace Acme;\nclass Gone {}\n",
		"vendor/acme/lib/README.php":     "<?php\nclass NotAutoloaded {}\n",
	})
	patched := "<?php\nnamespace Acme;\n\nclass Widget { public function patched() {} }\n"
	writeFiles(t, root, map[string]string{"vendor/acme/lib/src/Widget.php": patched})
	if err := os.Remove(filepath.Join(root, "vendor/acme/lib/src/Gone.php")); err != nil {
		t.Fatal(err)
	}
	out := refreshPaths(t, ws, root, "vendor/acme/lib/src/Widget.php", "vendor/acme/lib/src/Gone.php", "vendor/acme/lib/README.php")
	if len(out.Files) != 2 {
		t.Errorf("both vendor files changed: %v", out.Files)
	}
	class := ws.Index.Lookup(`Acme\Widget`)
	if class == nil || class.Source != symbols.SourceVendor || class.Range.Start.Line != 3 {
		t.Errorf("the class is the vendor file's new text, read from disk: %+v", class)
	}
	if !textIsReadFromDisk(t, ws, root, "vendor/acme/lib/src/Widget.php") {
		t.Error("the re-indexed vendor file's text stays on disk")
	}
	if ws.Index.Lookup(`Acme\Gone`) != nil {
		t.Error("a vendor file deleted on disk leaves the index")
	}
	if ws.Index.Lookup("NotAutoloaded") != nil {
		t.Error("a vendor file Composer doesn't load stays out of the index")
	}
}

func textIsReadFromDisk(t *testing.T, ws *Bootstrapped, root, name string) bool {
	t.Helper()
	later := "<?php\n// written after indexing\n"
	writeFiles(t, root, map[string]string{name: later})
	return ws.Index.GetFileSource(symbols.PathToURI(filepath.Join(root, name))) == later
}

func TestAVendorFileTheEditorReleasesIsReindexedFromDisk(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{
		"vendor/composer/installed.json": `{"packages": [{"name": "acme/lib", "install-path": "../acme/lib", "autoload": {"psr-4": {"Acme\\": "src/"}}}]}`,
		"vendor/acme/lib/src/Widget.php": "<?php\nnamespace Acme;\nclass Widget {}\n",
	})
	widget := symbols.PathToURI(filepath.Join(root, "vendor/acme/lib/src/Widget.php"))
	ws.IndexFile(widget, "<?php\nnamespace Acme;\nclass Widget {}\nclass EditedWidget {}\n")
	if class := ws.Index.Lookup(`Acme\Widget`); class == nil || class.Source != symbols.SourceProject {
		t.Fatalf("the editor's text is indexed as a project file while it is open: %+v", class)
	}

	if out := refreshPaths(t, ws, root, "vendor/acme/lib/src/Widget.php"); len(out.Files) != 1 {
		t.Errorf("the released vendor file is re-indexed: %v", out.Files)
	}
	if class := ws.Index.Lookup(`Acme\Widget`); class == nil || class.Source != symbols.SourceVendor {
		t.Errorf("the class is a vendor class again: %+v", class)
	}
	if ws.Index.Lookup(`Acme\EditedWidget`) != nil {
		t.Error("the editor's unsaved text leaves the index")
	}
	if !textIsReadFromDisk(t, ws, root, "vendor/acme/lib/src/Widget.php") {
		t.Error("the vendor file's text is read from disk again instead of kept")
	}
}

func TestAVendorFileGoneFromDiskAndIndexIsNotRemovedAgain(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{
		"vendor/composer/installed.json": `{"packages": [{"name": "acme/lib", "install-path": "../acme/lib", "autoload": {"psr-4": {"Acme\\": "src/"}}}]}`,
		"vendor/acme/lib/src/Gone.php":   "<?php\nnamespace Acme;\nclass Gone {}\n",
	})
	if err := os.Remove(filepath.Join(root, "vendor/acme/lib/src/Gone.php")); err != nil {
		t.Fatal(err)
	}
	if out := refreshPaths(t, ws, root, "vendor/acme/lib/src/Gone.php"); len(out.Files) != 1 {
		t.Fatalf("the deleted vendor file leaves the index: %v", out.Files)
	}
	if out := refreshPaths(t, ws, root, "vendor/acme/lib/src/Gone.php"); len(out.Files) != 0 {
		t.Errorf("a second event for the deleted file changes nothing: %v", out.Files)
	}
}
