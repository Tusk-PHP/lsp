package composer

import (
	"reflect"
	"testing"
)

func TestDirectRequires(t *testing.T) {
	root := t.TempDir()
	writeComposerJSON(t, root, `{
		"require": {
			"vendor/a": "^1.0",
			"vendor/b": "^2.0",
			"php": ">=8.1",
			"ext-json": "*",
			"php-64bit": "*"
		},
		"require-dev": {
			"phpunit/phpunit": "^10"
		}
	}`)

	got := DirectRequires(root)
	want := []string{"vendor/a", "vendor/b"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DirectRequires() = %v, want %v", got, want)
	}
}

func TestDirectRequiresMissingFile(t *testing.T) {
	got := DirectRequires(t.TempDir())
	if got != nil {
		t.Fatalf("DirectRequires() = %v, want nil", got)
	}
}

func TestDirectRequiresMalformedJSON(t *testing.T) {
	root := t.TempDir()
	writeComposerJSON(t, root, `{ not valid json }`)

	got := DirectRequires(root)
	if got != nil {
		t.Fatalf("DirectRequires() = %v, want nil", got)
	}
}

func TestDirectRequiresExcludesPlatformPackages(t *testing.T) {
	root := t.TempDir()
	writeComposerJSON(t, root, `{
		"require": {
			"php": "^8.0",
			"PHP": "^8.0",
			"ext-mbstring": "*",
			"EXT-JSON": "*",
			"php-64bit": "*",
			"PHP-ZEND-ENGINE": "*",
			"myvendor/mypkg": "^1.0"
		}
	}`)

	got := DirectRequires(root)
	want := []string{"myvendor/mypkg"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DirectRequires() = %v, want %v", got, want)
	}
}

func TestDirectRequiresEmptyRequire(t *testing.T) {
	root := t.TempDir()
	writeComposerJSON(t, root, `{
		"require": {},
		"require-dev": {
			"phpunit/phpunit": "^10"
		}
	}`)

	got := DirectRequires(root)
	// An empty require block returns a nil slice (no elements appended).
	if len(got) != 0 {
		t.Fatalf("DirectRequires() = %v, want empty/nil", got)
	}
}
