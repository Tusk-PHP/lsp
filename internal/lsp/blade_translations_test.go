package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const bladeTranslations = `<?php

return [
    'welcome' => 'Welcome',
    'apples' => 'One apple|Many apples',
];
`

func bladeTranslationWorkspace(t *testing.T, view string) (h *lspHarness, viewURI, langPath string) {
	t.Helper()
	root := t.TempDir()
	viewPath := filepath.Join(root, "resources", "views", "home.blade.php")
	langPath = filepath.Join(root, "lang", "en", "messages.php")
	files := map[string]string{
		filepath.Join(root, "composer.json"): `{"require": {"laravel/framework": "^11.0"}}`,
		langPath:                             bladeTranslations,
		viewPath:                             view,
	}
	for path, text := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h = newHarness(t)
	h.readResponse(h.send("initialize", map[string]interface{}{
		"rootUri": "file://" + root, "capabilities": map[string]interface{}{}, "processId": nil,
	}))
	h.notify("initialized", map[string]interface{}{})
	time.Sleep(500 * time.Millisecond)
	viewURI = "file://" + filepath.ToSlash(viewPath)
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": viewURI, "languageId": "blade", "version": 1, "text": view},
	})
	time.Sleep(200 * time.Millisecond)
	return h, viewURI, langPath
}

func TestBladeTranslationDirectivesComplete(t *testing.T) {
	view := "<h1>@lang('messages.')</h1>\n<p>@choice(\n    'messages.',\n    $count\n)</p>\n"
	h, viewURI, _ := bladeTranslationWorkspace(t, view)
	defer h.close()

	for _, at := range []map[string]interface{}{
		posOf(t, view, "@lang('messages.", len("@lang('messages.")),
		posOf(t, view, "    'messages.", len("    'messages.")),
	} {
		var items []struct{ Label string }
		request(t, h, "textDocument/completion", viewURI, at, nil, &items)
		labels := make([]string, 0, len(items))
		for _, it := range items {
			labels = append(labels, it.Label)
		}
		joined := strings.Join(labels, ",")
		if !strings.Contains(joined, "messages.welcome") || !strings.Contains(joined, "messages.apples") {
			t.Errorf("at %v: got %v", at, labels)
		}
	}
}

func TestBladeTranslationDirectivesGoToDefinition(t *testing.T) {
	view := "<h1>@lang('messages.welcome')</h1>\n<p>@choice('messages.apples', $count)</p>\n<p>@lang(['name' => 'messages.welcome']) :name @endlang</p>\n"
	h, viewURI, langPath := bladeTranslationWorkspace(t, view)
	defer h.close()

	for needle, line := range map[string]int{"messages.welcome')": 3, "messages.apples": 4} {
		var loc testLocation
		request(t, h, "textDocument/definition", viewURI, posOf(t, view, needle, 10), nil, &loc)
		if !strings.HasSuffix(loc.URI, filepath.ToSlash(langPath)) || loc.Range.Start.Line != line {
			t.Errorf("%s: got %+v, want line %d of %s", needle, loc, line, langPath)
		}
	}

	var none interface{}
	request(t, h, "textDocument/definition", viewURI, posOf(t, view, "messages.welcome']", 10), nil, &none)
	if none != nil {
		t.Errorf("the replacements of a block @lang are not a key: got %v", none)
	}
}
