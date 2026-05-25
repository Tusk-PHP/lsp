package stubs

import (
	"reflect"
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
)

func TestBuiltinPHPReturnsDeterministicEntries(t *testing.T) {
	entriesA, err := BuiltinPHP()
	if err != nil {
		t.Fatalf("BuiltinPHP() error = %v", err)
	}

	entriesB, err := BuiltinPHP()
	if err != nil {
		t.Fatalf("BuiltinPHP() second call error = %v", err)
	}

	if !reflect.DeepEqual(entriesA, entriesB) {
		t.Fatal("BuiltinPHP() returned non-deterministic entries")
	}

	if len(entriesA) == 0 {
		t.Fatal("BuiltinPHP() returned no entries")
	}

	for i := 1; i < len(entriesA); i++ {
		if entriesA[i-1].Path > entriesA[i].Path {
			t.Fatalf("entries out of order: %q before %q", entriesA[i-1].Path, entriesA[i].Path)
		}
	}
}

func TestBuiltinPHPEntriesAreParseable(t *testing.T) {
	entries, err := BuiltinPHP()
	if err != nil {
		t.Fatalf("BuiltinPHP() error = %v", err)
	}

	for _, entry := range entries {
		file := parser.ParseFile(entry.Content)
		if file == nil {
			t.Fatalf("ParseFile(%q) returned nil", entry.Path)
		}
	}
}

func TestBuiltinPHPForProfileSelectsCoreVersionLayers(t *testing.T) {
	php74 := mustEntries(t, Profile{PHPVersion: "7.4"})
	if !hasEntry(php74, "php/core/php-7.4.php") {
		t.Fatal("expected PHP 7.4 core baseline")
	}
	if hasEntry(php74, "php/core/php-8.0.php") {
		t.Fatal("did not expect PHP 8.0 core delta for PHP 7.4 profile")
	}
	if containsSource(php74, "str_contains") {
		t.Fatal("did not expect str_contains in PHP 7.4 profile")
	}

	php80 := mustEntries(t, Profile{PHPVersion: "8.0"})
	if !hasEntry(php80, "php/core/php-8.0.php") {
		t.Fatal("expected PHP 8.0 core delta")
	}
	if !containsSource(php80, "str_contains") {
		t.Fatal("expected str_contains in PHP 8.0 profile")
	}
}

func TestBuiltinPHPForProfileSelectsExtensionLayers(t *testing.T) {
	withoutJSON := mustEntries(t, Profile{PHPVersion: "8.3"})
	if hasEntry(withoutJSON, "php/extensions/json/php-7.4.php") {
		t.Fatal("did not expect json extension without ext-json profile")
	}

	json74 := mustEntries(t, Profile{PHPVersion: "7.4", Extensions: []string{"json"}})
	if !containsSource(json74, "json_decode") {
		t.Fatal("expected json_decode with ext-json PHP 7.4 profile")
	}
	if containsSource(json74, "json_validate") {
		t.Fatal("did not expect json_validate before PHP 8.3")
	}

	json83 := mustEntries(t, Profile{PHPVersion: "8.3", Extensions: []string{"ext-json"}})
	if !containsSource(json83, "json_validate") {
		t.Fatal("expected json_validate with ext-json PHP 8.3 profile")
	}
}

func mustEntries(t *testing.T, profile Profile) []Entry {
	t.Helper()
	entries, err := BuiltinPHPForProfile(profile)
	if err != nil {
		t.Fatalf("BuiltinPHPForProfile() error = %v", err)
	}
	return entries
}

func hasEntry(entries []Entry, path string) bool {
	for _, entry := range entries {
		if entry.Path == path {
			return true
		}
	}
	return false
}

func containsSource(entries []Entry, needle string) bool {
	for _, entry := range entries {
		if strings.Contains(entry.Content, needle) {
			return true
		}
	}
	return false
}
