package completion

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func setupEnvKeysProvider(tb testing.TB, files map[string]string) *Provider {
	tb.Helper()
	root := tb.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	idx := symbols.NewIndex()
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, root, "laravel"), "laravel")
	p.SetEnvKeys(frameworklaravel.NewEnvKeys(root))
	return p
}

func TestEnvKeysAreCompletedInsideEnv(t *testing.T) {
	p := setupEnvKeysProvider(t, map[string]string{
		".env":         "APP_NAME=Shop\nAPP_KEY=base64:secret\nDB_HOST=127.0.0.1\n",
		".env.example": "APP_NAME=Laravel\nAPP_URL=http://localhost\n",
	})
	for _, line := range []string{
		"return ['name' => env('APP_')];",
		"return ['name' => \\Illuminate\\Support\\Env::get('APP_')];",
	} {
		t.Run(line, func(t *testing.T) {
			source := "<?php\n" + line + "\n"
			character := strings.Index(line, "APP_") + len("APP_")
			items := p.GetCompletions("file:///config/app.php", source, protocol.Position{Line: 1, Character: character})
			var labels []string
			for _, item := range items {
				labels = append(labels, item.Label)
			}
			if want := []string{"APP_KEY", "APP_NAME", "APP_URL"}; !slices.Equal(labels, want) {
				t.Fatalf("labels = %v, want %v", labels, want)
			}
			start := strings.Index(line, "APP_")
			for _, item := range items {
				want := protocol.TextEdit{Range: protocol.Range{
					Start: protocol.Position{Line: 1, Character: start}, End: protocol.Position{Line: 1, Character: character},
				}, NewText: item.Label}
				if item.TextEdit == nil || *item.TextEdit != want {
					t.Errorf("%s replaces what was typed: %+v, want %+v", item.Label, item.TextEdit, want)
				}
				if strings.Contains(item.Detail, "Shop") || strings.Contains(item.Detail, "secret") || strings.Contains(item.Documentation, "secret") {
					t.Errorf("%s shows no value: %+v", item.Label, item)
				}
			}
			if items[1].Detail != ".env, .env.example" {
				t.Errorf("APP_NAME's detail names its files: %q", items[1].Detail)
			}
		})
	}
}

func TestEnvKeysAreNotCompletedOutsideTheKeyArgument(t *testing.T) {
	p := setupEnvKeysProvider(t, map[string]string{".env": "APP_NAME=Shop\n"})
	for _, line := range []string{"env('APP_NAME', 'A');", "config('A');", "$x = 'A';"} {
		source := "<?php\n" + line + "\n"
		character := strings.LastIndex(line, "A") + 1
		for _, item := range p.GetCompletions("file:///a.php", source, protocol.Position{Line: 1, Character: character}) {
			if item.Label == "APP_NAME" {
				t.Errorf("%s: no env key completion", line)
			}
		}
	}
}

func BenchmarkEnvKeyCompletion(b *testing.B) {
	var env strings.Builder
	for i := range 300 {
		env.WriteString("SERVICE_")
		env.WriteString(strings.Repeat("X", i%7))
		env.WriteString("_KEY_")
		env.WriteString(string(rune('A' + i%26)))
		env.WriteString(strings.Repeat("Y", i/26))
		env.WriteString("=value\n")
	}
	p := setupEnvKeysProvider(b, map[string]string{".env": env.String(), ".env.example": env.String()})
	source := "<?php\nreturn [\n    'key' => env('SERVICE_'),\n];\n"
	pos := protocol.Position{Line: 2, Character: len("    'key' => env('SERVICE_")}
	if items := p.GetCompletions("file:///config/services.php", source, pos); len(items) != 300 {
		b.Fatalf("%d items", len(items))
	}
	b.ReportAllocs()
	for b.Loop() {
		p.GetCompletions("file:///config/services.php", source, pos)
	}
}
