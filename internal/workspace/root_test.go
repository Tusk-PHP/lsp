package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRootAndPaths(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "A.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	links := t.TempDir()
	link := filepath.Join(links, "app")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	fileLink := filepath.Join(dir, "src", "B.php")
	if err := os.Symlink(filepath.Join(dir, "src", "A.php"), fileLink); err != nil {
		t.Fatal(err)
	}

	if got, err := ResolveRoot(link); err != nil || got != dir {
		t.Errorf("ResolveRoot(%s) = %q, %v; want %s", link, got, err, dir)
	}
	for _, missing := range []string{filepath.Join(links, "missing"), filepath.Join(link, "src", "A.php")} {
		if got, err := ResolveRoot(missing); err == nil {
			t.Errorf("ResolveRoot(%s) = %q, want an error", missing, got)
		}
	}
	for given, want := range map[string]string{
		filepath.Join(link, "src"):          filepath.Join(dir, "src"),
		filepath.Join(link, "src", "A.php"): filepath.Join(dir, "src", "A.php"),
		filepath.Join(link, "src", "B.php"): fileLink,
	} {
		if got, err := ResolvePath(given); err != nil || got != want {
			t.Errorf("ResolvePath(%s) = %q, %v; want %s", given, got, err, want)
		}
	}
	if got, err := ResolvePath(filepath.Join(link, "src", "C.php")); err == nil {
		t.Errorf("ResolvePath of a missing file = %q, want an error", got)
	}
}
