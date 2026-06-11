package symfony

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectSymfonyTranslationContext(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		cursor  int
		wantKey string
		wantPart string
	}{
		{
			name:     "translator->trans",
			line:     `$translator->trans('user.profile.title')`,
			cursor:   len(`$translator->trans('user.pro`),
			wantKey:  "user.profile.title",
			wantPart: "user.pro",
		},
		{
			name:     "t() shorthand",
			line:     `t('nav.home')`,
			cursor:   len(`t('nav`),
			wantKey:  "nav.home",
			wantPart: "nav",
		},
		{
			name:     "TranslatableMessage constructor",
			line:     `new TranslatableMessage('button.save')`,
			cursor:   len(`new TranslatableMessage('button.sa`),
			wantKey:  "button.save",
			wantPart: "button.sa",
		},
		{
			name:     "this->trans",
			line:     `$this->trans('validators.field.required')`,
			cursor:   len(`$this->trans('validators`),
			wantKey:  "validators.field.required",
			wantPart: "validators",
		},
		{
			name:    "cursor outside string returns nil",
			line:    `$translator->trans('key')`,
			cursor:  2,
			wantKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := DetectSymfonyTranslationContext(tt.line, tt.cursor)
			if tt.wantKey == "" {
				if ctx != nil {
					t.Fatalf("expected nil context, got %+v", ctx)
				}
				return
			}
			if ctx == nil {
				t.Fatal("expected context, got nil")
			}
			if ctx.Key != tt.wantKey {
				t.Errorf("Key = %q, want %q", ctx.Key, tt.wantKey)
			}
			if ctx.Partial != tt.wantPart {
				t.Errorf("Partial = %q, want %q", ctx.Partial, tt.wantPart)
			}
		})
	}
}

func TestTranslationResolverYAML(t *testing.T) {
	root := setupSymfonyTranslationFixture(t)
	r := NewTranslationResolver(root)

	// Test completion with no partial — should return all entries
	ctx := &TranslationContext{Partial: ""}
	items := r.Complete(ctx)
	if len(items) == 0 {
		t.Fatal("expected completion items")
	}

	labels := map[string]bool{}
	for _, item := range items {
		labels[item.Label] = true
	}

	for _, key := range []string{"user.profile.title", "user.profile.bio", "nav.home", "button.save"} {
		if !labels[key] {
			t.Errorf("expected key %q in completions", key)
		}
	}
}

func TestTranslationResolverYAMLPartialFilter(t *testing.T) {
	root := setupSymfonyTranslationFixture(t)
	r := NewTranslationResolver(root)

	ctx := &TranslationContext{Partial: "user."}
	items := r.Complete(ctx)
	for _, item := range items {
		if !strings.HasPrefix(item.Label, "user.") {
			t.Errorf("unexpected key %q not starting with 'user.'", item.Label)
		}
	}
	if len(items) == 0 {
		t.Fatal("expected items matching user. prefix")
	}
}

func TestTranslationResolverDomainFilter(t *testing.T) {
	root := setupSymfonyTranslationFixture(t)
	r := NewTranslationResolver(root)

	// Filter by validators domain
	ctx := &TranslationContext{Partial: "", Domain: "validators"}
	items := r.Complete(ctx)
	for _, item := range items {
		if !strings.HasPrefix(item.Label, "field.") {
			t.Errorf("unexpected key %q for validators domain", item.Label)
		}
	}
	if len(items) == 0 {
		t.Fatal("expected items for validators domain")
	}
}

func TestTranslationResolverDefinition(t *testing.T) {
	root := setupSymfonyTranslationFixture(t)
	r := NewTranslationResolver(root)

	loc := r.Definition("user.profile.title")
	if loc == nil {
		t.Fatal("expected definition for user.profile.title")
	}
	if !strings.Contains(loc.URI, "messages.en.yaml") {
		t.Errorf("expected messages.en.yaml, got %s", loc.URI)
	}
}

func TestTranslationResolverXLIFF(t *testing.T) {
	root := setupSymfonyTranslationFixture(t)
	r := NewTranslationResolver(root)

	loc := r.Definition("welcome_message")
	if loc == nil {
		t.Fatal("expected definition for welcome_message (from xlf)")
	}
	if !strings.Contains(loc.URI, ".xlf") {
		t.Errorf("expected .xlf source, got %s", loc.URI)
	}
}

func TestTranslationResolverPHPFile(t *testing.T) {
	root := setupSymfonyTranslationFixture(t)
	r := NewTranslationResolver(root)

	loc := r.Definition("login.title")
	if loc == nil {
		t.Fatal("expected definition for login.title (from php)")
	}
	if !strings.Contains(loc.URI, ".php") {
		t.Errorf("expected .php source, got %s", loc.URI)
	}
}

func TestTranslationResolverMissingDir(t *testing.T) {
	// A root without a translations directory should return nil without panicking.
	r := NewTranslationResolver(t.TempDir())
	ctx := &TranslationContext{Partial: ""}
	items := r.Complete(ctx)
	if items != nil {
		t.Errorf("expected nil for missing translations dir, got %v", items)
	}
}

func TestTranslationResolverMalformedXLIFF(t *testing.T) {
	root := t.TempDir()
	translationsDir := filepath.Join(root, "translations")
	os.MkdirAll(translationsDir, 0755)
	// Write malformed XML — should not panic
	os.WriteFile(filepath.Join(translationsDir, "messages.en.xlf"), []byte(`not valid xml <<<`), 0644)

	r := NewTranslationResolver(root)
	ctx := &TranslationContext{Partial: ""}
	items := r.Complete(ctx) // must not panic
	_ = items
}

func TestParseSymfonyTranslationFilename(t *testing.T) {
	tests := []struct {
		input          string
		wantDomain     string
		wantLocale     string
	}{
		{"messages.en.yaml", "messages", "en"},
		{"validators.fr.yml", "validators", "fr"},
		{"security.en.xlf", "security", "en"},
		{"messages.en.xliff", "messages", "en"},
		{"security.en.php", "security", "en"},
		{"messages.yaml", "messages", "en"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, l := parseSymfonyTranslationFilename(tt.input)
			if d != tt.wantDomain {
				t.Errorf("domain = %q, want %q", d, tt.wantDomain)
			}
			if l != tt.wantLocale {
				t.Errorf("locale = %q, want %q", l, tt.wantLocale)
			}
		})
	}
}

func TestParseYAMLTranslationNestedKeys(t *testing.T) {
	yaml := `user:
    profile:
        title: User Profile
        bio: Biography
button:
    save: Save
`
	entries := parseYAMLTranslationContent(yaml, "messages", "en", "messages.en.yaml", "file:///messages.en.yaml")
	labels := map[string]bool{}
	for _, e := range entries {
		labels[e.Key] = true
	}
	for _, key := range []string{"user.profile.title", "user.profile.bio", "button.save"} {
		if !labels[key] {
			t.Errorf("missing key %q", key)
		}
	}
}

func TestParseXLIFFContent(t *testing.T) {
	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
    <file source-language="en" datatype="plaintext">
        <body>
            <trans-unit id="hello">
                <source>hello</source>
                <target>Hello!</target>
            </trans-unit>
        </body>
    </file>
</xliff>`
	entries := parseXLIFFContent(xliff, "messages", "en", "messages.en.xlf", "file:///messages.en.xlf")
	if len(entries) == 0 {
		t.Fatal("expected entries from XLIFF")
	}
	if entries[0].Key != "hello" {
		t.Errorf("key = %q, want 'hello'", entries[0].Key)
	}
	if entries[0].Domain != "messages" {
		t.Errorf("domain = %q", entries[0].Domain)
	}
}

// setupSymfonyTranslationFixture creates a temp directory with a Symfony-style
// translations/ directory containing YAML, XLIFF, and PHP fixtures.
func setupSymfonyTranslationFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	translationsDir := filepath.Join(root, "translations")
	if err := os.MkdirAll(translationsDir, 0755); err != nil {
		t.Fatalf("mkdir translations: %v", err)
	}

	files := map[string]string{
		"messages.en.yaml": `user:
    profile:
        title: User Profile
        bio: Biography
    login:
        button: Log In
nav:
    home: Home
    about: About Us
button:
    save: Save
    cancel: Cancel
`,
		"validators.en.yaml": `field:
    required: This field is required.
    min_length: Too short.
`,
		"messages.en.xlf": `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
    <file source-language="en" datatype="plaintext">
        <body>
            <trans-unit id="welcome_message">
                <source>welcome_message</source>
                <target>Welcome!</target>
            </trans-unit>
        </body>
    </file>
</xliff>`,
		"security.en.php": `<?php
return [
    'login' => [
        'title' => 'Login',
        'username' => 'Username',
    ],
    'logout' => 'Logout',
];
`,
	}

	for name, content := range files {
		path := filepath.Join(translationsDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}
