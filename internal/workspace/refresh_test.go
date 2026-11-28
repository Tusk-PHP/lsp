package workspace

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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
	out, err := ws.Refresh(Refresh{Paths: paths, Config: func() (*config.Config, error) {
		return config.LoadFromFile(ws.ConfigPath())
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
		EditorOwns: func(path string) bool { return path == filepath.Join(root, "app/Open.php") },
	})
	if err != nil || len(out.Files) != 0 || ws.Index.Lookup("FromDisk") != nil || ws.Index.Lookup("Open") == nil {
		t.Errorf("an open document keeps the editor's text: %+v %v", out, err)
	}
}

func TestRefreshReportsDocumentsTheEditorOpensWhileTheyAreReadFromDisk(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{"app/Open.php": "<?php\nclass Open {}\n"})
	writeFiles(t, root, map[string]string{"app/Open.php": "<?php\nclass FromDisk {}\n", "app/Closed.php": "<?php\nclass Closed {}\n"})
	open := filepath.Join(root, "app/Open.php")
	asked := 0
	out, err := ws.Refresh(Refresh{
		Paths: []string{open, filepath.Join(root, "app/Closed.php")},
		EditorOwns: func(path string) bool {
			if path != open {
				return false
			}
			asked++
			return asked > 1
		},
	})
	if err != nil || len(out.EditorOpened) != 1 || out.EditorOpened[0] != symbols.PathToURI(open) {
		t.Errorf("a document opened while its disk text was indexed is handed back to the editor: %+v %v", out, err)
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

func TestRefreshSeparatesConfigErrorsFromFilesItCouldNotRead(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{"app/Order.php": "<?php\nclass Order {}\n"})
	writeFiles(t, root, map[string]string{"app/Locked.php": "<?php\nclass Locked {}\n", "app/Added.php": "<?php\nclass Added {}\n"})
	locked := filepath.Join(root, "app/Locked.php")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })
	broken := errors.New("unexpected end of JSON input")
	out, err := ws.Refresh(Refresh{
		Paths:  []string{locked, filepath.Join(root, "app/Added.php"), ws.ConfigPath()},
		Config: func() (*config.Config, error) { return nil, broken },
	})
	var configErr *ConfigError
	if !errors.As(err, &configErr) || !errors.Is(err, broken) {
		t.Errorf("a config that can't be loaded is a ConfigError: %v", err)
	}
	if len(out.Failed) != 1 || out.Failed[0] != locked {
		t.Errorf("the unreadable file is listed as failed: %v", out.Failed)
	}
	if ws.Index.Lookup("Added") == nil {
		t.Error("the other files are re-indexed")
	}
}

func TestWatchGlobsCoverWhatARefreshReads(t *testing.T) {
	globs := WatchGlobs()
	for _, want := range []string{"**/*.php", "**/*.phtml", "**/composer.json", "**/composer.lock", "**/vendor/composer/installed.json", "**/" + config.FileName} {
		if !slices.Contains(globs, want) {
			t.Errorf("the watchers cover %s: %v", want, globs)
		}
	}
}

func TestRefreshFollowsDirectoriesCreatedAndDeleted(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{
		"app/Billing/Invoice.php":  "<?php\nnamespace App;\nclass Invoice {}\n",
		"app/Billing/Tax/Rate.php": "<?php\nnamespace App;\nclass Rate {}\n",
	})
	if err := os.Rename(filepath.Join(root, "app/Billing"), filepath.Join(root, "app/Finance")); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, map[string]string{"vendor/acme/lib/Lib.php": "<?php\nclass VendorLib {}\n"})

	out := refreshPaths(t, ws, root, "app/Billing", "app/Finance", "vendor/acme/lib")
	if len(out.Files) != 4 {
		t.Errorf("two files left the old directory and two arrived in the new one: %v", out.Files)
	}
	for class, file := range map[string]string{`App\Invoice`: "app/Finance/Invoice.php", `App\Rate`: "app/Finance/Tax/Rate.php"} {
		if sym := ws.Index.Lookup(class); sym == nil || sym.URI != symbols.PathToURI(filepath.Join(root, file)) {
			t.Errorf("%s is declared at the new place: %+v", class, sym)
		}
	}
	if ws.Index.Lookup("VendorLib") != nil {
		t.Error("a directory under excludePaths stays out")
	}
}

func TestRefreshReportsWhatItCannotReadAndIndexesTheRest(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions don't stop this user from reading")
	}
	ws, root := newRefreshWorkspace(t, nil, map[string]string{"app/Kept.php": "<?php\nclass Kept {}\n"})
	writeFiles(t, root, map[string]string{
		"app/Locked.php":            "<?php\nclass Locked {}\n",
		"app/Sealed/Inner/Deep.php": "<?php\nclass Deep {}\n",
		"app/Mixed/Ok.php":          "<?php\nclass Ok {}\n",
		"app/Mixed/Shut/Hidden.php": "<?php\nclass Hidden {}\n",
	})
	for _, name := range []string{"app/Locked.php", "app/Sealed", "app/Mixed/Shut"} {
		path := filepath.Join(root, name)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
	}

	_, err := ws.Refresh(Refresh{Paths: []string{
		filepath.Join(root, "app/Locked.php"),
		filepath.Join(root, "app/Sealed/Inner"),
		filepath.Join(root, "app/Mixed"),
	}})
	if err == nil {
		t.Fatal("a file and a directory that can't be read are reported")
	}
	for _, want := range []string{"re-index " + filepath.Join(root, "app/Locked.php"), "look at " + filepath.Join(root, "app/Sealed/Inner")} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error says %q: %v", want, err)
		}
	}
	if ws.Index.Lookup("Ok") == nil {
		t.Error("the readable file of a directory with an unreadable subdirectory is indexed")
	}
	if ws.Index.Lookup("Locked") != nil || ws.Index.Lookup("Hidden") != nil {
		t.Error("nothing unreadable is indexed")
	}
}

func TestRefreshDuringTheFirstWalkSharesItsFileLimit(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"a.php": "<?php\nclass A {}\n", "b.php": "<?php\nclass B {}\n"})
	cfg := config.DefaultConfig()
	cfg.MaxIndexFiles = 1
	ws := New(Options{RootPath: root, Framework: "none", Config: cfg, Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})

	refreshPaths(t, ws, root, "a.php", "b.php")
	if ws.Index.Lookup("A") == nil || ws.Index.Lookup("B") != nil {
		t.Error("before the walk ends, a refresh admits files up to maxIndexFiles")
	}
	if err := os.Remove(filepath.Join(root, "a.php")); err != nil {
		t.Fatal(err)
	}
	refreshPaths(t, ws, root, "a.php")
	refreshPaths(t, ws, root, "b.php")
	if ws.Index.Lookup("A") != nil || ws.Index.Lookup("B") == nil {
		t.Error("a file deleted before the walk ends frees its place")
	}
}

func TestRefreshKeepsLaravelsViewsRoutesAndIDEHelperCurrent(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"composer.json":  `{"name": "app/app"}`,
		"routes/web.php": "<?php\nRoute::get('/', fn () => 1)->name('home');\n",
	})
	ws := New(Options{RootPath: root, Framework: "laravel", Config: config.DefaultConfig(), Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	ws.IndexWorkspace()
	if err := ws.ScanRoutes(); err != nil {
		t.Fatal(err)
	}
	if ws.RouteIndex.Find("home") == nil {
		t.Fatal("the route file is read at startup")
	}

	writeFiles(t, root, map[string]string{
		"routes/web.php":                    "<?php\nRoute::get('/', fn () => 1)->name('dashboard');\n",
		"resources/views/welcome.blade.php": "<h1>{{ $title }}</h1>\n",
		"_ide_helper.php":                   "<?php\nnamespace Illuminate\\Support\\Facades {\nclass Route {}\n}\n",
	})
	refreshPaths(t, ws, root, "routes/web.php", "resources/views/welcome.blade.php", "_ide_helper.php")
	if ws.RouteIndex.Find("home") != nil || ws.RouteIndex.Find("dashboard") == nil {
		t.Error("a renamed route is found under its new name only")
	}
	if !ws.Index.HasFile(symbols.PathToURI(filepath.Join(root, "resources/views/welcome.blade.php"))) {
		t.Error("a new Blade view is listed for references and rename")
	}
	if sym := ws.Index.Lookup(`Illuminate\Support\Facades\Route`); sym == nil || sym.URI != symbols.PathToURI(filepath.Join(root, "_ide_helper.php")) {
		t.Errorf("the IDE helper's classes are read: %+v", sym)
	}

	if err := os.Remove(filepath.Join(root, "routes/web.php")); err != nil {
		t.Fatal(err)
	}
	refreshPaths(t, ws, root, "routes/web.php")
	if ws.RouteIndex.Find("dashboard") != nil {
		t.Error("a deleted route file's routes leave")
	}
}

func TestRefreshUnderAPathWithASpaceKeepsOneURIPerFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my app")
	writeFiles(t, root, map[string]string{"app/Order.php": "<?php\nnamespace App;\nclass Order {}\n", "app/Open.php": "<?php\nnamespace App;\nclass Open {}\n"})
	ws := New(Options{RootPath: root, Framework: "none", Config: config.DefaultConfig(), Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	ws.IndexWorkspace()

	order, open := filepath.Join(root, "app", "Order.php"), filepath.Join(root, "app", "Open.php")
	writeFiles(t, root, map[string]string{"app/Order.php": "<?php\nnamespace App;\nclass Order { public function total(): int { return 0; } }\n", "app/Open.php": "<?php\nnamespace App;\nclass FromDisk {}\n"})
	out, err := ws.Refresh(Refresh{Paths: []string{order, open}, EditorOwns: func(path string) bool { return path == open }})
	if err != nil || !slices.Equal(out.Files, []string{symbols.PathToURI(order)}) {
		t.Errorf("only the file the editor doesn't own is re-indexed, under the URI the walk gave it: %+v %v", out, err)
	}
	if defs := ws.Index.Definitions(`App\Order`); len(defs) != 1 || defs[0].URI != symbols.PathToURI(order) {
		t.Errorf("Order is declared once, at %s: %+v", symbols.PathToURI(order), defs)
	}
	if ws.Index.Lookup(`App\FromDisk`) != nil {
		t.Error("the open file's disk text isn't indexed")
	}
}

func TestIDEHelperUnderAPathWithASpaceIsIndexedUnderItsFileURI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my app")
	writeFiles(t, root, map[string]string{"_ide_helper.php": "<?php\nnamespace Illuminate\\Support\\Facades {\nclass Route {}\n}\n"})
	ws := New(Options{RootPath: root, Framework: "none", Config: config.DefaultConfig(), Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	ws.IndexWorkspace()
	want := symbols.PathToURI(filepath.Join(root, "_ide_helper.php"))
	if route := ws.Index.Lookup(`Illuminate\Support\Facades\Route`); route == nil || route.URI != want {
		t.Errorf("the IDE helper's class is declared at %s: %+v", want, route)
	}
}
