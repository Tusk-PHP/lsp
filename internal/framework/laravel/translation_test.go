package laravel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectTranslationContext(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		cursor int
		key    string
		part   string
	}{
		{
			name:   "php helper",
			line:   "{{ __('messages.we') }}",
			cursor: len("{{ __('messages.we"),
			key:    "messages.we",
			part:   "messages.we",
		},
		{
			name:   "blade directive",
			line:   "@lang('Welcome')",
			cursor: len("@lang('Wel"),
			key:    "Welcome",
			part:   "Wel",
		},
		{
			name:   "trans choice",
			line:   "trans_choice('messages.count', $count)",
			cursor: len("trans_choice('messages.count"),
			key:    "messages.count",
			part:   "messages.count",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := DetectTranslationContext(tt.line, tt.cursor)
			if ctx == nil {
				t.Fatal("expected translation context")
			}
			if ctx.Key != tt.key {
				t.Fatalf("Key = %q, want %q", ctx.Key, tt.key)
			}
			if ctx.Partial != tt.part {
				t.Fatalf("Partial = %q, want %q", ctx.Partial, tt.part)
			}
		})
	}
}

func TestTranslationResolverPrefersConfiguredLocale(t *testing.T) {
	root := setupTranslationFixture(t)
	resolver := NewTranslationResolver(root)

	loc := resolver.Definition("messages.welcome")
	if loc == nil {
		t.Fatal("expected definition")
	}
	if !strings.Contains(loc.URI, "/lang/es/messages.php") {
		t.Fatalf("expected es locale definition, got %s", loc.URI)
	}
}

func TestTranslationResolverDiscoversPHPAndJSONKeys(t *testing.T) {
	root := setupTranslationFixture(t)
	resolver := NewTranslationResolver(root)

	ctx := &TranslationStringContext{Partial: "mes"}
	items := resolver.Complete(ctx)
	labels := map[string]bool{}
	for _, item := range items {
		labels[item.Label] = true
	}

	for _, key := range []string{"messages.welcome", "messages.auth.failed"} {
		if !labels[key] {
			t.Fatalf("expected key %q in completions", key)
		}
	}

	jsonLoc := resolver.Definition("Welcome")
	if jsonLoc == nil {
		t.Fatal("expected json translation definition")
	}
	if !strings.Contains(jsonLoc.URI, "/lang/es.json") {
		t.Fatalf("expected es json translation, got %s", jsonLoc.URI)
	}
}

func setupTranslationFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	mkdir := func(rel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, rel), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
	}
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	mkdir("config")
	mkdir(filepath.Join("lang", "en"))
	mkdir(filepath.Join("lang", "es"))

	write("config/app.php", `<?php
return [
    'locale' => 'es',
];
`)
	write(filepath.Join("lang", "en", "messages.php"), `<?php
return [
    'welcome' => 'Welcome',
    'auth' => [
        'failed' => 'These credentials do not match our records.',
    ],
];
`)
	write(filepath.Join("lang", "es", "messages.php"), `<?php
return [
    'welcome' => 'Bienvenido',
];
`)
	write(filepath.Join("lang", "en.json"), "{\n  \"Welcome\": \"Welcome\"\n}\n")
	write(filepath.Join("lang", "es.json"), "{\n  \"Welcome\": \"Bienvenido\"\n}\n")

	return root
}
