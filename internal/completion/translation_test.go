package completion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
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

func TestSymfonyTranslationCompletion(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "translations")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "messages.en.yaml"), []byte("user:\n    title: Title\nnav:\n    home: Home\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "validators.en.yaml"), []byte("user:\n    required: Required\n"), 0644); err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, root, "symfony"), "symfony")
	p.SetSymfonyTranslationResolver(frameworksymfony.NewTranslationResolver(root))

	source := "<?php\n$translator->trans('user.ti"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: len("$translator->trans('user.ti")})
	var got []string
	for _, item := range items {
		got = append(got, item.Label+" | "+item.InsertText+" | "+item.Detail)
	}
	want := []string{"user.title | tle | messages (en) - messages.en.yaml"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTranslationItemInsertsTheRestOfTheKey(t *testing.T) {
	for _, c := range []struct{ key, partial, want string }{
		{"messages.welcome", "mes", "sages.welcome"},
		{"Messages.welcome", "mes", "Messages.welcome"},
		{"nav.home", "", "nav.home"},
	} {
		if got := translationItem(c.key, c.partial, "").InsertText; got != c.want {
			t.Errorf("translationItem(%q, %q).InsertText = %q, want %q", c.key, c.partial, got, c.want)
		}
	}
}
