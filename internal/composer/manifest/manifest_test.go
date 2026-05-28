package manifest

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestParseRequireBlocks(t *testing.T) {
	source := `{
  "name": "acme/widgets",
  "require": {
    "php": "^8.1",
    "laravel/framework": "^10.0"
  },
  "require-dev": {
    "phpunit/phpunit": "^10.0"
  }
}`
	m := Parse(source)

	wantNames := []string{"php", "laravel/framework", "phpunit/phpunit"}
	if len(m.Requires) != len(wantNames) {
		t.Fatalf("got %d requires, want %d: %+v", len(m.Requires), len(wantNames), m.Requires)
	}
	for i, want := range wantNames {
		if m.Requires[i].Name != want {
			t.Errorf("Requires[%d].Name = %q, want %q", i, m.Requires[i].Name, want)
		}
	}
	if m.Requires[0].Kind != RequireKindRuntime {
		t.Errorf("php should be RequireKindRuntime, got %v", m.Requires[0].Kind)
	}
	if m.Requires[2].Kind != RequireKindDev {
		t.Errorf("phpunit should be RequireKindDev, got %v", m.Requires[2].Kind)
	}
	if m.Requires[1].Value != "^10.0" {
		t.Errorf("laravel/framework value = %q, want ^10.0", m.Requires[1].Value)
	}
}

func TestPackageAtCursorOnName(t *testing.T) {
	source := `{
  "require": {
    "laravel/framework": "^10.0"
  }
}`
	m := Parse(source)
	// Cursor on "laravel/framework"
	// line 2 is `    "laravel/framework": "^10.0"`
	// content starts after the opening quote
	pos := find(source, "laravel/framework")
	got := m.PackageAtCursor(protocol.Position{Line: pos.Line, Character: pos.Character + 3})
	if got == nil {
		t.Fatalf("expected to hit a package, got nil")
	}
	if got.Name != "laravel/framework" {
		t.Errorf("got %q, want laravel/framework", got.Name)
	}
}

func TestPackageAtCursorOnValue(t *testing.T) {
	source := `{
  "require": {
    "laravel/framework": "^10.0"
  }
}`
	m := Parse(source)
	pos := find(source, "^10.0")
	got := m.PackageAtCursor(protocol.Position{Line: pos.Line, Character: pos.Character + 1})
	if got == nil || got.Name != "laravel/framework" {
		t.Fatalf("expected laravel/framework via value hover, got %+v", got)
	}
}

func TestPackageAtCursorOutsideReturnsNil(t *testing.T) {
	source := `{
  "name": "acme/widgets",
  "require": {
    "laravel/framework": "^10.0"
  }
}`
	m := Parse(source)
	// Cursor on the "name" value — should NOT match.
	pos := find(source, "acme/widgets")
	if got := m.PackageAtCursor(protocol.Position{Line: pos.Line, Character: pos.Character + 1}); got != nil {
		t.Errorf("expected nil for cursor outside require, got %+v", got)
	}
}

func TestRepositories(t *testing.T) {
	source := `{
  "repositories": [
    { "type": "vcs", "url": "https://github.com/acme/fork.git" },
    { "type": "path", "url": "../local-pkg" }
  ],
  "require": { "acme/fork": "dev-main" }
}`
	m := Parse(source)
	if len(m.Repositories) != 2 {
		t.Fatalf("expected 2 repositories, got %d: %+v", len(m.Repositories), m.Repositories)
	}
	if m.Repositories[0].Type != "vcs" || !strings.HasPrefix(m.Repositories[0].URL, "https://github.com/") {
		t.Errorf("unexpected repo[0]: %+v", m.Repositories[0])
	}
	if m.Repositories[1].Type != "path" {
		t.Errorf("unexpected repo[1]: %+v", m.Repositories[1])
	}
}

func TestParseMalformedDoesNotPanic(t *testing.T) {
	bads := []string{
		``,
		`{`,
		`{ "require": { "foo": `,
		`{ "require": { "foo/bar": "^1.0", }`,    // trailing comma
		`{ "require": { "broken": "missing-end`, // unterminated
	}
	for _, src := range bads {
		_ = Parse(src) // must not panic
	}
}

// find returns the 0-based line/col where needle first appears in source.
func find(source, needle string) protocol.Position {
	idx := strings.Index(source, needle)
	if idx < 0 {
		return protocol.Position{}
	}
	line := 0
	col := 0
	for i := 0; i < idx; i++ {
		if source[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return protocol.Position{Line: line, Character: col}
}
