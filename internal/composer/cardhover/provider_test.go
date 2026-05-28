package cardhover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestHoverRendersLinkedTitleAndConstraint(t *testing.T) {
	root := writeFixture(t, `{
  "require": {
    "laravel/framework": "^10.0"
  }
}`, "")
	p := NewProvider(root, "8.2")

	source := readComposer(t, root)
	pos := findPos(source, "laravel/framework", 3)
	h := p.Hover("composer.json", source, pos)
	if h == nil {
		t.Fatal("expected hover, got nil")
	}
	body := h.Contents.Value
	if !strings.Contains(body, "[laravel/framework](https://packagist.org/packages/laravel/framework)") {
		t.Errorf("missing linked title in:\n%s", body)
	}
	if !strings.Contains(body, "`^10.0`") {
		t.Errorf("missing constraint in:\n%s", body)
	}
}

func TestHoverWithLockfileShowsDescription(t *testing.T) {
	root := writeFixture(t, `{
  "require": {
    "laravel/framework": "^10.0"
  }
}`, `{
  "packages": [
    {
      "name": "laravel/framework",
      "version": "v10.48.4",
      "description": "The Laravel Framework.",
      "license": ["MIT"],
      "require": {"php": "^8.1"},
      "source": {"url": "https://github.com/laravel/framework.git", "type": "git"}
    }
  ]
}`)
	p := NewProvider(root, "8.2")
	source := readComposer(t, root)
	pos := findPos(source, "laravel/framework", 3)

	h := p.Hover("composer.json", source, pos)
	if h == nil {
		t.Fatal("expected hover, got nil")
	}
	body := h.Contents.Value
	for _, want := range []string{
		"The Laravel Framework.",
		"Requires PHP ^8.1 ✓",
		"project targets 8.2",
		"Installed: `v10.48.4`",
		"License: MIT",
		"Source: https://github.com/laravel/framework.git",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("hover missing %q\n--- body ---\n%s", want, body)
		}
	}
}

func TestHoverWithIncompatiblePHPShowsCross(t *testing.T) {
	root := writeFixture(t, `{
  "require": {
    "modern/lib": "^1.0"
  }
}`, `{
  "packages": [
    {"name": "modern/lib", "version": "1.0.0", "require": {"php": ">=8.2"}}
  ]
}`)
	p := NewProvider(root, "7.4")
	source := readComposer(t, root)
	pos := findPos(source, "modern/lib", 3)

	h := p.Hover("composer.json", source, pos)
	if h == nil {
		t.Fatal("expected hover, got nil")
	}
	if !strings.Contains(h.Contents.Value, "✗") {
		t.Errorf("expected red cross for incompatible PHP, got:\n%s", h.Contents.Value)
	}
}

func TestHoverPlatformConstraintReturnsNil(t *testing.T) {
	// Cursor on "php" itself — that's a platform constraint, not a package.
	root := writeFixture(t, `{
  "require": {
    "php": "^8.1"
  }
}`, "")
	p := NewProvider(root, "8.2")
	source := readComposer(t, root)
	pos := findPos(source, `"php"`, 1)

	if h := p.Hover("composer.json", source, pos); h != nil {
		t.Errorf("expected nil hover on platform constraint, got %+v", h)
	}
}

func TestHoverOutsideRequireReturnsNil(t *testing.T) {
	root := writeFixture(t, `{
  "name": "acme/widgets",
  "require": {
    "laravel/framework": "^10.0"
  }
}`, "")
	p := NewProvider(root, "8.2")
	source := readComposer(t, root)
	pos := findPos(source, "acme/widgets", 1)
	if h := p.Hover("composer.json", source, pos); h != nil {
		t.Errorf("expected nil hover for cursor on project name, got %+v", h)
	}
}

func TestHoverRespectsRepositoriesOverride(t *testing.T) {
	root := writeFixture(t, `{
  "repositories": [
    {"type": "vcs", "url": "git@github.com:acme/fork.git"}
  ],
  "require": {
    "acme/fork": "dev-main"
  }
}`, "")
	p := NewProvider(root, "8.2")
	source := readComposer(t, root)
	pos := findPos(source, `"acme/fork":`, 2)
	h := p.Hover("composer.json", source, pos)
	if h == nil {
		t.Fatal("expected hover, got nil")
	}
	if !strings.Contains(h.Contents.Value, "(https://github.com/acme/fork)") {
		t.Errorf("expected GitHub link override, got:\n%s", h.Contents.Value)
	}
}

func TestHoverDisabledReturnsNil(t *testing.T) {
	root := writeFixture(t, `{"require":{"laravel/framework":"^10.0"}}`, "")
	p := NewProvider(root, "8.2")
	p.SetEnabled(false)
	source := readComposer(t, root)
	pos := findPos(source, "laravel/framework", 3)
	if h := p.Hover("composer.json", source, pos); h != nil {
		t.Errorf("expected nil when disabled, got %+v", h)
	}
}

func TestDefinitionOffByDefault(t *testing.T) {
	root := writeFixture(t, `{"require":{"laravel/framework":"^10.0"}}`, "")
	p := NewProvider(root, "8.2")
	source := readComposer(t, root)
	pos := findPos(source, "laravel/framework", 3)
	if got := p.Definition("composer.json", source, pos); got.ExternalURL != "" {
		t.Errorf("expected empty URL when openOnDefinition disabled, got %q", got.ExternalURL)
	}
}

func TestDefinitionOnWhenEnabled(t *testing.T) {
	root := writeFixture(t, `{"require":{"laravel/framework":"^10.0"}}`, "")
	p := NewProvider(root, "8.2")
	p.SetOpenOnDefinition(true)
	source := readComposer(t, root)
	pos := findPos(source, "laravel/framework", 3)
	got := p.Definition("composer.json", source, pos)
	want := "https://packagist.org/packages/laravel/framework"
	if got.ExternalURL != want {
		t.Errorf("got %q, want %q", got.ExternalURL, want)
	}
}

func TestIsComposerJSON(t *testing.T) {
	cases := map[string]bool{
		"file:///workspace/composer.json":              true,
		"file:///workspace/vendor/foo/bar/composer.json": true,
		"file:///workspace/package.json":               false,
		"/some/path/composer.json":                     true,
		"/some/path/notcomposer.json":                  false,
	}
	for uri, want := range cases {
		if got := IsComposerJSON(uri); got != want {
			t.Errorf("IsComposerJSON(%q) = %v, want %v", uri, got, want)
		}
	}
}

// --- helpers ---

func writeFixture(t *testing.T, composerJSON, lockJSON string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(composerJSON), 0644); err != nil {
		t.Fatalf("write composer.json: %v", err)
	}
	if lockJSON != "" {
		if err := os.WriteFile(filepath.Join(root, "composer.lock"), []byte(lockJSON), 0644); err != nil {
			t.Fatalf("write composer.lock: %v", err)
		}
	}
	return root
}

func readComposer(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		t.Fatalf("read composer.json: %v", err)
	}
	return string(b)
}

// findPos returns a position offsetCols into the first occurrence of
// needle in source. Useful to land the cursor inside a quoted token.
func findPos(source, needle string, offsetCols int) protocol.Position {
	idx := strings.Index(source, needle)
	if idx < 0 {
		panic("needle not found: " + needle)
	}
	line, col := 0, 0
	for i := 0; i < idx; i++ {
		if source[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return protocol.Position{Line: line, Character: col + offsetCols}
}
