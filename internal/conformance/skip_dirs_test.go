//go:build conformance

package conformance

import (
	"path/filepath"
	"testing"
)

func TestSkippedTestdataDir(t *testing.T) {
	root := filepath.Join("x", "testdata")
	cases := map[string]bool{
		"laravel/vendor":     true,
		"symfony/vendor":     true,
		"field/_repos":       true,
		"corpus/_repos":      true,
		"laravel":            false,
		"laravel/app":        false,
		"project/src/vendor": false,
		"laravel/vendor/foo": false,
		"field":              false,
	}
	for rel, want := range cases {
		if got := skippedTestdataDir(root, filepath.Join(root, filepath.FromSlash(rel))); got != want {
			t.Errorf("%s: got %v, want %v", rel, got, want)
		}
	}
}
