package completion

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestLaravelTranslationCompletionInPHPHelper(t *testing.T) {
	p := setupTranslationProvider(t)

	source := "<?php\n__('mes"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: len("__('mes")})
	labels := collectLabels(items)

	if !labels["messages.welcome"] {
		t.Fatalf("expected messages.welcome, got %v", mapKeys(labels, 10))
	}

	for _, item := range items {
		if item.Label == "messages.welcome" {
			if item.InsertText != "sages.welcome" {
				t.Fatalf("InsertText = %q, want %q", item.InsertText, "sages.welcome")
			}
			return
		}
	}
	t.Fatal("expected translation completion item")
}

func TestLaravelTranslationCompletionInBladeDirective(t *testing.T) {
	p := setupTranslationProvider(t)

	source := "@lang('Wel"
	items := p.GetCompletions("file:///test.blade.php", source, protocol.Position{Line: 0, Character: len("@lang('Wel")})
	labels := collectLabels(items)

	if !labels["Welcome"] {
		t.Fatalf("expected Welcome, got %v", mapKeys(labels, 10))
	}
}

func setupTranslationProvider(t *testing.T) *Provider {
	t.Helper()

	root := setupTranslationProject(t)
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, root, "laravel")
	p := NewProvider(idx, ca, "laravel")
	p.SetTranslationResolver(frameworklaravel.NewTranslationResolver(root))
	return p
}

func setupTranslationProject(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for _, dir := range []string{
		"config",
		filepath.Join("lang", "en"),
		filepath.Join("lang", "es"),
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	files := map[string]string{
		filepath.Join("config", "app.php"): `<?php
return [
    'locale' => 'es',
];
`,
		filepath.Join("lang", "en", "messages.php"): `<?php
return [
    'welcome' => 'Welcome',
    'auth' => [
        'failed' => 'These credentials do not match our records.',
    ],
];
`,
		filepath.Join("lang", "es", "messages.php"): `<?php
return [
    'welcome' => 'Bienvenido',
];
`,
		filepath.Join("lang", "en.json"): "{\n  \"Welcome\": \"Welcome\"\n}\n",
		filepath.Join("lang", "es.json"): "{\n  \"Welcome\": \"Bienvenido\"\n}\n",
	}

	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	return root
}
