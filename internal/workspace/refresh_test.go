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

func newRefreshWorkspace(t *testing.T, cfg *config.Config, files map[string]string) (*Bootstrapped, string) {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, files)
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	ws := New(Options{RootPath: root, Framework: "none", Config: cfg, Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	ws.IndexWorkspace()
	ws.IndexComposerDependencies()
	return ws, root
}

func refreshPaths(t *testing.T, ws *Bootstrapped, root string, names ...string) Refreshed {
	t.Helper()
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(root, name)
	}
	out, err := ws.Refresh(Refresh{Paths: paths, Config: func() *config.Config {
		cfg, err := config.LoadFromFile(ws.ConfigPath())
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRefreshFollowsProjectFilesOnDisk(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{
		"app/Old.php":     "<?php\nnamespace App;\nclass Old {}\n",
		"app/Changed.php": "<?php\nnamespace App;\nclass Changed {}\n",
	})
	if err := os.Remove(filepath.Join(root, "app/Old.php")); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, map[string]string{
		"app/New.php":            "<?php\nnamespace App;\nclass New {}\n",
		"app/Changed.php":        "<?php\nnamespace App;\nclass Renamed {}\n",
		"vendor/acme/Vendor.php": "<?php\nclass VendorClass {}\n",
		"app/notes.txt":          "not PHP",
	})
	out := refreshPaths(t, ws, root, "app/Old.php", "app/New.php", "app/Changed.php", "app/Changed.php", "vendor/acme/Vendor.php", "app/notes.txt", "app/Gone.php")
	if len(out.Files) != 3 || out.Composer || out.ProfileChanged {
		t.Errorf("three project files changed: %+v", out)
	}
	if ws.Index.Lookup(`App\Old`) != nil || ws.Index.Lookup(`App\Changed`) != nil {
		t.Error("the deleted file and the old content leave the index")
	}
	if ws.Index.Lookup(`App\New`) == nil || ws.Index.Lookup(`App\Renamed`) == nil {
		t.Error("the new file and the new content are indexed")
	}
	if ws.Index.Lookup("VendorClass") != nil {
		t.Error("a file under excludePaths stays out")
	}
}

func TestRefreshSkipsDocumentsTheEditorOwns(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{"app/Open.php": "<?php\nclass Open {}\n"})
	writeFiles(t, root, map[string]string{"app/Open.php": "<?php\nclass FromDisk {}\n"})
	out, err := ws.Refresh(Refresh{
		Paths:      []string{filepath.Join(root, "app/Open.php")},
		EditorOwns: func(uri string) bool { return uri == "file://"+filepath.Join(root, "app/Open.php") },
	})
	if err != nil || len(out.Files) != 0 || ws.Index.Lookup("FromDisk") != nil || ws.Index.Lookup("Open") == nil {
		t.Errorf("an open document keeps the editor's text: %+v %v", out, err)
	}
}

func TestRefreshRespectsMaxIndexFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MaxIndexFiles = 1
	ws, root := newRefreshWorkspace(t, cfg, map[string]string{"a.php": "<?php\nclass A {}\n"})
	writeFiles(t, root, map[string]string{"b.php": "<?php\nclass B {}\n"})
	refreshPaths(t, ws, root, "b.php")
	if ws.Index.Lookup("B") != nil {
		t.Error("a new file past maxIndexFiles isn't indexed")
	}
	writeFiles(t, root, map[string]string{"a.php": "<?php\nclass A2 {}\n"})
	refreshPaths(t, ws, root, "a.php")
	if ws.Index.Lookup("A2") == nil {
		t.Error("a file already indexed is re-indexed at the limit")
	}
	if err := os.Remove(filepath.Join(root, "a.php")); err != nil {
		t.Fatal(err)
	}
	refreshPaths(t, ws, root, "a.php", "b.php")
	if ws.Index.Lookup("A2") != nil || ws.Index.Lookup("B") == nil {
		t.Error("a deleted file frees its place for a new one")
	}
}

func TestRefreshResolvesTheProfileAgain(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{"composer.json": `{"require": {"php": "^8.1", "ext-redis": "*"}}`})
	if _, err := ws.SetProfile(symbols.BuiltinProfile{PHPVersion: "8.1", Extensions: []string{"redis"}}); err != nil {
		t.Fatal(err)
	}
	if out := refreshPaths(t, ws, root, "composer.json"); out.ProfileChanged || !out.Composer {
		t.Errorf("an unchanged platform leaves the builtins alone: %+v", out)
	}

	writeFiles(t, root, map[string]string{"composer.json": `{"require": {"php": "^7.4"}}`})
	out := refreshPaths(t, ws, root, "composer.json")
	if !out.ProfileChanged || out.Profile.PHPVersion != "7.4" || out.ProfileSource != "composer" || ws.Profile().PHPVersion != "7.4" {
		t.Fatalf("composer.json's new PHP version is the profile's: %+v", out)
	}
	if ws.Index.Lookup("str_contains") != nil || ws.Index.Lookup("Redis") != nil {
		t.Error("PHP 8 builtins and the dropped extension leave the index")
	}

	writeFiles(t, root, map[string]string{"composer.json": `{"name": "app/app"}`, ".tusk-php.json": `{"phpVersion": "8.2"}`})
	refreshPaths(t, ws, root, ".tusk-php.json")
	if ws.Profile().PHPVersion != "8.2" || ws.Index.Lookup("str_contains") == nil {
		t.Errorf("the config file's PHP version applies when composer.json names none: %+v", ws.Profile())
	}
}

func TestRefreshReadsComposersAutoloadMapAgain(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{
		"composer.json":                  `{"name": "app/app"}`,
		"vendor/composer/installed.json": `{"packages": []}`,
	})
	if _, ok := ws.ClassLoader().Load(`Acme\Widget`); ok {
		t.Fatal("no package provides the class yet")
	}
	writeFiles(t, root, map[string]string{
		"vendor/composer/installed.json": `{"packages": [{"name": "acme/lib", "install-path": "../acme/lib", "autoload": {"psr-4": {"Acme\\": "src/"}}}]}`,
		"vendor/acme/lib/src/Widget.php": "<?php\nnamespace Acme;\nclass Widget {}\n",
	})
	if out := refreshPaths(t, ws, root, "vendor/composer/installed.json"); !out.Composer {
		t.Errorf("installed.json is Composer state: %+v", out)
	}
	if _, ok := ws.ClassLoader().Load(`Acme\Widget`); !ok {
		t.Error("the class loader finds a package Composer installed after startup")
	}
}
