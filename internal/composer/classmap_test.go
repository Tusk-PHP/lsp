package composer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetAutoloadPathsIncludesVendorClassmap(t *testing.T) {
	root := t.TempDir()
	composerDir := filepath.Join(root, "vendor", "composer")
	if err := os.MkdirAll(composerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	installed := `{"packages":[{"name":"phpunit/phpunit","install-path":"../phpunit/phpunit",
		"autoload":{"classmap":["src/","extra/Single.php"]}}]}`
	if err := os.WriteFile(filepath.Join(composerDir, "installed.json"), []byte(installed), 0o644); err != nil {
		t.Fatal(err)
	}

	pkgDir := filepath.Join(root, "vendor", "phpunit", "phpunit")
	want := map[string]bool{
		filepath.Join(pkgDir, "src"):              false, // directory
		filepath.Join(pkgDir, "extra/Single.php"): true,  // file
	}
	found := 0
	for _, e := range GetAutoloadPaths(root) {
		isFile, ok := want[e.Path]
		if !ok {
			continue
		}
		found++
		if !e.IsVendor || e.Namespace != "" || e.IsFile != isFile {
			t.Errorf("unexpected classmap entry %+v", e)
		}
	}
	if found != len(want) {
		t.Fatalf("found %d of %d classmap entries", found, len(want))
	}
}

func TestGetAutoloadPathsIncludesPSR0AndComposerRuntime(t *testing.T) {
	root := t.TempDir()
	composerDir := filepath.Join(root, "vendor", "composer")
	if err := os.MkdirAll(composerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	installed := `{"packages":[{"name":"magento/zend-db","install-path":"../magento/zend-db",
		"autoload":{"psr-0":{"Zend_Db":"library/","Twig\\":["lib/","lib/"]}}}]}`
	for name, body := range map[string]string{"installed.json": installed, "InstalledVersions.php": "<?php", "ClassLoader.php": "<?php"} {
		if err := os.WriteFile(filepath.Join(composerDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pkgDir := filepath.Join(root, "vendor", "magento", "zend-db")
	want := map[string]bool{ // path → is a file
		filepath.Join(pkgDir, "library", "Zend", "Db"):      false,
		filepath.Join(pkgDir, "library", "Zend", "Db.php"):  true,
		filepath.Join(pkgDir, "lib", "Twig"):                false,
		filepath.Join(pkgDir, "lib", "Twig.php"):            true,
		filepath.Join(composerDir, "InstalledVersions.php"): true,
		filepath.Join(composerDir, "ClassLoader.php"):       true,
	}
	seen := map[string]int{}
	for _, e := range GetAutoloadPaths(root) {
		isFile, ok := want[e.Path]
		if !ok {
			continue
		}
		seen[e.Path]++
		if !e.IsVendor || e.IsFile != isFile {
			t.Errorf("unexpected entry %+v", e)
		}
	}
	for path := range want {
		if seen[path] != 1 {
			t.Errorf("%s listed %d times, want once", path, seen[path])
		}
	}
}
