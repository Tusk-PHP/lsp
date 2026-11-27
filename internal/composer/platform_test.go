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
	if got.PHP != nil {
		t.Fatalf("expected no PHP constraint, got %+v", got.PHP)
	}
	if len(got.Extensions) != 0 {
		t.Fatalf("expected no extensions, got %v", got.Extensions)
	}
}

func TestGetPlatformIncludesSuggestedExtensions(t *testing.T) {
	root := t.TempDir()
	writeComposerJSON(t, root, `{
		"require": {"php": "^7.4", "ext-redis": "*"},
		"require-dev": {"ext-xdebug": "*"},
		"suggest": {
			"ext-imagick": "For image processing (not a version constraint)",
			"ext-redis": "For caching",
			"ext-apcu": "",
			"php": "^8.5",
			"vendor/package": "Optional integration"
		},
		"config": {"platform": {"ext-redis": "6.0", "ext-ssh2": "1.4"}}
	}`)
	got := GetPlatform(root)
	if got.PHP == nil || got.PHP.Oldest.String() != "7.4" {
		t.Fatalf("suggest changed PHP target: %+v", got.PHP)
	}
	want := []string{"apcu", "imagick", "redis", "ssh2"}
	if !reflect.DeepEqual(got.Extensions, want) {
		t.Fatalf("extensions = %v, want %v", got.Extensions, want)
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
	if got.PHP == nil || got.PHP.Oldest.String() != "8.1" {
		t.Fatalf("expected PHPVersion 8.1, got %+v", got.PHP)
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

	if got.PHP == nil || got.PHP.Oldest.String() != "7.4" {
		t.Fatalf("expected PHPVersion 7.4, got %+v", got.PHP)
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

func TestParsePHPConstraint(t *testing.T) {
	tests := []struct {
		constraint string
		oldest     string
		grammars   string
	}{
		{"^7.4", "7.4", "7.4"},
		{"~7.4.0", "7.4", "7.4"},
		{"~7.4", "7.4", "7.4"},
		{"7.4.*", "7.4", "7.4"},
		{">=7.4", "7.4", "7.4-8.6"},
		{"^7.4 || ^8.0", "7.4", "7.4-8.6"},
		{"^7.4|^8.0", "7.4", "7.4-8.6"},
		{">=8.1", "8.1", "8.1-8.6"},
		{"8.1.0", "8.1", "8.1"},
		{">=7.2.24", "7.2", "7.4-8.6"},
		{">=7.4 <8.2", "7.4", "7.4-8.1"},
		{">=7.4, <=8.2.5", "7.4", "7.4-8.2"},
		{">= 8.0 < 8.3.1", "8.0", "8.0-8.3"},
		{"^8.2", "8.2", "8.2-8.6"},
		{"~8.2.0", "8.2", "8.2"},
		{"8.1 - 8.3", "8.1", "8.1-8.3"},
		{"8.1 - 8.3.0", "8.1", "8.1-8.3"},
		{"^7.4@dev", "7.4", "7.4"},
		{"*", "0.0", "7.4-8.6"},
		{"^9.0", "9.0", "8.6"},
		{"^7.4 || >=8.3 <8.1", "7.4", "7.4"},
		{">=8.1 <8.3 || >=8.5", "8.1", "8.1-8.6"},
		{">=7.4 <8.2 <8.4", "7.4", "7.4-8.1"},
		{">=7.4 !=8.0", "7.4", "7.4-8.6"},
		{"^0.3", "0.3", "7.4"},
		{"X", "0.0", "7.4-8.6"},
		{"8.1.2.3", "8.1", "8.1"},
	}
	for _, tt := range tests {
		got, ok := ParsePHPConstraint(tt.constraint)
		if !ok || got.Oldest.String() != tt.oldest || got.Grammars.String() != tt.grammars {
			t.Errorf("ParsePHPConstraint(%q) = %+v, %v; want oldest %s, grammars %s", tt.constraint, got, ok, tt.oldest, tt.grammars)
		}
	}
	for _, constraint := range []string{"", "dev-main", "~", ">=x", ">=8.2 <8.0", "8.1 - dev", "=>8.0"} {
		if got, ok := ParsePHPConstraint(constraint); ok {
			t.Errorf("ParsePHPConstraint(%q) = %+v, want no version", constraint, got)
		}
	}
}
