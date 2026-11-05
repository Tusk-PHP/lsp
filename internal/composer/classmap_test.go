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
