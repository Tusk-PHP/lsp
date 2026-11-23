package completion

import (
	"os"
	"path/filepath"
	"testing"
)

// requireLaravelVendor skips a test that needs testdata/laravel's Composer
// dependencies when they aren't installed, as in a fresh clone or worktree.
func requireLaravelVendor(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join("..", "..", "testdata", "laravel", "vendor", "laravel", "framework")); err != nil {
		t.Skip("testdata/laravel/vendor is missing: run `composer install --working-dir testdata/laravel`")
	}
}

// requireSymfonyVendor skips a test that needs testdata/symfony's Composer
// dependencies when they aren't installed; only its installed.json is in git.
func requireSymfonyVendor(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join("..", "..", "testdata", "symfony", "vendor", "symfony", "framework-bundle")); err != nil {
		t.Skip("testdata/symfony/vendor is missing: run `composer install --working-dir testdata/symfony`")
	}
}
