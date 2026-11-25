package workspace

import (
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
)

func TestExcludeRulesMatchNamesAnywhereAndPathsFromTheRoot(t *testing.T) {
	rules := newExcludeRules([]string{"vendor", "var/cache", "./build/out/", "/tmp/generated", "/dist", "./coverage", "logs/", "", "."})
	cases := map[string]bool{
		"dist":              true,
		"src/dist":          false,
		"coverage":          true,
		"app/coverage":      false,
		"logs":              true,
		"app/logs":          true,
		"vendor":            true,
		"src/vendor":        true,
		"var/cache":         true,
		"var":               false,
		"var/log":           false,
		"app/var/cache":     false,
		"cache":             false,
		"build/out":         true,
		"build":             false,
		"tmp/generated":     true,
		"src/tmp/generated": false,
		"src":               false,
	}
	for rel, want := range cases {
		if got := rules.excludes(rel); got != want {
			t.Errorf("excludes(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestExcludeRulesReachOnlyFilesOutsideExcludedDirectories(t *testing.T) {
	rules := newExcludeRules([]string{"vendor", "var/cache"})
	cases := map[string]bool{
		"src/Kernel.php":              true,
		"Kernel.php":                  true,
		"var/cache/dev/Container.php": false,
		"var/Thing.php":               true,
		"src/vendor/lib/Thing.php":    false,
		"src/var/cache/Thing.php":     true,
		"var/cache_old/Thing.php":     true,
	}
	for rel, want := range cases {
		if got := rules.reaches(rel); got != want {
			t.Errorf("reaches(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestIndexWorkspaceSkipsTheDefaultVarCache(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"src/Kernel.php":              "<?php\nnamespace App;\nclass Kernel {}\n",
		"var/cache/dev/Container.php": "<?php\nnamespace ContainerAbc;\nclass Container {}\n",
		"src/var/cache/Kept.php":      "<?php\nnamespace App;\nclass Kept {}\n",
	})
	ws := New(Options{RootPath: root, Config: config.DefaultConfig()})

	if count := ws.IndexWorkspace().Count; count != 2 {
		t.Errorf("indexed %d files, want 2", count)
	}
	if ws.Index.Lookup(`ContainerAbc\Container`) != nil {
		t.Error("var/cache at the root is excluded by the default excludePaths")
	}
	if ws.Index.Lookup(`App\Kept`) == nil {
		t.Error("an entry with a slash matches from the root only, so src/var/cache is indexed")
	}
	if ws.walkerReaches(filepath.Join(root, "var", "cache", "dev", "Container.php")) {
		t.Error("the class loader treats var/cache as excluded too")
	}
}

func TestIndexWorkspaceIndexesARootNamedLikeAnExcludedDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "storage")
	writeFiles(t, root, map[string]string{"src/Thing.php": "<?php\nclass Thing {}\n"})
	ws := New(Options{RootPath: root, Config: config.DefaultConfig()})

	if count := ws.IndexWorkspace().Count; count != 1 {
		t.Errorf("indexed %d files, want 1: excludePaths match below the root, not the root itself", count)
	}
}
