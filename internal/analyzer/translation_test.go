package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	frameworklaravel "github.com/open-southeners/tusk-php/internal/framework/laravel"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func TestLaravelTranslationDefinitionFromPHPHelper(t *testing.T) {
	a := setupTranslationAnalyzer(t)

	source := "<?php\n__('messages.welcome');\n"
	pos := charPosOf(t, source, "welcome", "__('messages.welcome')")
	loc := a.FindDefinition("file:///test.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition")
	}
	if !strings.Contains(loc.URI, "/lang/es/messages.php") {
		t.Fatalf("expected es messages definition, got %s", loc.URI)
	}
}

func TestLaravelTranslationDefinitionFromBladeDirective(t *testing.T) {
	a := setupTranslationAnalyzer(t)

	source := "@lang('Welcome')"
	pos := charPosOf(t, source, "Welcome", "@lang('Welcome')")
	loc := a.FindDefinition("file:///test.blade.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition")
	}
	if !strings.Contains(loc.URI, "/lang/es.json") {
		t.Fatalf("expected es json definition, got %s", loc.URI)
	}
}

func setupTranslationAnalyzer(t *testing.T) *Analyzer {
	t.Helper()

	root := setupTranslationAnalyzerProject(t)
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, root, "laravel")
	a := NewAnalyzer(idx, ca)
	a.SetTranslationResolver(frameworklaravel.NewTranslationResolver(root))
	return a
}

func setupTranslationAnalyzerProject(t *testing.T) string {
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
