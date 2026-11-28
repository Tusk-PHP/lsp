package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestBladeProviderRegistrations(t *testing.T) {
	root := t.TempDir()
	view := "@admin\n    <x-ui::button size=\"lg\" />\n@endadmin\n@admin\n"
	files := map[string]string{
		"artisan":       "",
		"composer.json": `{"autoload": {"psr-4": {"App\\": "app/"}}}`,
		"app/Providers/AppServiceProvider.php": `<?php
namespace App\Providers;

use Illuminate\Support\Facades\Blade;

class AppServiceProvider
{
    public function boot(): void
    {
        Blade::if('admin', fn () => true);
        Blade::anonymousComponentPath(resource_path('views/ui'), 'ui');
    }
}
`,
		"resources/views/ui/button.blade.php": "@props(['size' => 'md', 'variant' => 'primary'])\n<button>{{ $slot }}</button>\n",
		"resources/views/page.blade.php":      view,
	}
	for rel, text := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": symbols.PathToURI(root), "capabilities": map[string]interface{}{}, "processId": nil}))
	h.notify("initialized", map[string]interface{}{})
	time.Sleep(800 * time.Millisecond)
	uri := symbols.PathToURI(filepath.Join(root, "resources", "views", "page.blade.php"))
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "blade", "version": 1, "text": view},
	})

	// The first @admin block balances; the second is reported, by its own name.
	deadline := time.Now().Add(8 * time.Second)
	var last []string
	for time.Now().Before(deadline) {
		msg := h.readNotification("textDocument/publishDiagnostics", 8*time.Second)
		raw, _ := json.Marshal(msg["params"])
		var params struct {
			URI         string
			Diagnostics []struct{ Code, Message string }
		}
		_ = json.Unmarshal(raw, &params)
		if params.URI != uri {
			continue
		}
		last = last[:0]
		for _, d := range params.Diagnostics {
			last = append(last, string(d.Code)+": "+d.Message)
		}
		if strings.Join(last, "\n") == "unclosed-directive: `@admin` is never closed with `@endadmin`" {
			break
		}
	}
	if strings.Join(last, "\n") != "unclosed-directive: `@admin` is never closed with `@endadmin`" {
		t.Fatalf("expected only the second @admin reported, got %q", last)
	}

	var locs []testLocation
	request(t, h, "textDocument/definition", uri, posOf(t, view, "ui::button", 1), nil, &locs)
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "resources/views/ui/button.blade.php") {
		t.Fatalf("expected the ui::button view, got %+v", locs)
	}
	var items []struct{ Label string }
	request(t, h, "textDocument/completion", uri, map[string]interface{}{"line": 1, "character": len("    <x-ui::button size=\"lg\" ")}, nil, &items)
	if len(items) != 1 || items[0].Label != "variant" {
		t.Fatalf("expected the variant attribute, got %+v", items)
	}
}
