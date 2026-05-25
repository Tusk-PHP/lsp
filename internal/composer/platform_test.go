package composer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetPlatformMissingComposer(t *testing.T) {
	got := GetPlatform(t.TempDir())

	if got.HasComposer {
		t.Fatal("expected HasComposer to be false")
	}
	if got.PHPVersion != "" {
		t.Fatalf("expected empty PHPVersion, got %q", got.PHPVersion)
	}
	if len(got.Extensions) != 0 {
		t.Fatalf("expected no extensions, got %v", got.Extensions)
	}
}

func TestGetPlatformUsesConfigPlatformPHPOverRequire(t *testing.T) {
	root := t.TempDir()
	writeComposerJSON(t, root, `{
		"require": {
			"php": "^8.2",
			"ext-json": "*",
			"ext-mbstring": "^1.0"
		},
		"config": {
			"platform": {
				"php": "^8.1",
				"ext-ctype": "8.1",
				"ext-mbstring": "8.1"
			}
		}
	}`)

	got := GetPlatform(root)

	if !got.HasComposer {
		t.Fatal("expected HasComposer to be true")
	}
	if got.PHPVersion != "8.1" {
		t.Fatalf("expected PHPVersion 8.1, got %q", got.PHPVersion)
	}

	wantExtensions := []string{"ctype", "json", "mbstring"}
	if !reflect.DeepEqual(got.Extensions, wantExtensions) {
		t.Fatalf("expected extensions %v, got %v", wantExtensions, got.Extensions)
	}
}

func TestGetPlatformFallsBackToRequirePHP(t *testing.T) {
	root := t.TempDir()
	writeComposerJSON(t, root, `{
		"require": {
			"php": ">=7.4",
			"ext-zlib": "*"
		}
	}`)

	got := GetPlatform(root)

	if got.PHPVersion != "7.4" {
		t.Fatalf("expected PHPVersion 7.4, got %q", got.PHPVersion)
	}
	wantExtensions := []string{"zlib"}
	if !reflect.DeepEqual(got.Extensions, wantExtensions) {
		t.Fatalf("expected extensions %v, got %v", wantExtensions, got.Extensions)
	}
}

func TestMinimumPHPVersion(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		want       string
	}{
		{name: "caret", constraint: "^7.4", want: "7.4"},
		{name: "greater-than-equal", constraint: ">=8.1", want: "8.1"},
		{name: "plain-version", constraint: "8.2.0", want: "8.2"},
		{name: "multiple-options", constraint: "^8.2 || ^8.1", want: "8.1"},
		{name: "major-only", constraint: "8", want: "8.0"},
		{name: "invalid", constraint: "dev-main", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minimumPHPVersion(tt.constraint)
			if got != tt.want {
				t.Fatalf("minimumPHPVersion(%q) = %q, want %q", tt.constraint, got, tt.want)
			}
		})
	}
}

func writeComposerJSON(t *testing.T, root, content string) {
	t.Helper()

	path := filepath.Join(root, "composer.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write composer.json: %v", err)
	}
}
