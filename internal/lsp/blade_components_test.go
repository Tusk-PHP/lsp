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

const componentClass = `<?php

namespace App\View\Components;

use Illuminate\View\Component;

class Alert extends Component
{
    public function __construct(public string $type = 'info', public string $alertTitle = '')
    {
    }

    public function render()
    {
        return view('components.alert');
    }
}
`

const componentPage = `<div>
    <x-alert type="warning" />
    <x-card title="Hi" />
    <x-missing />
</div>
`

// componentsWorkspace starts a server on a Laravel app with a class
// component, an anonymous one, and a page using them, and opens the page.
func componentsWorkspace(t *testing.T, page string) (h *lspHarness, root, uri string) {
	t.Helper()
	root = t.TempDir()
	files := map[string]string{
		"composer.json":                 `{"autoload": {"psr-4": {"App\\": "app/"}}}`,
		"artisan":                       "",
		"app/View/Components/Alert.php": componentClass,
		"resources/views/components/alert.blade.php":      "<div>{{ $slot }}</div>\n",
		"resources/views/components/card/index.blade.php": "@props(['title', 'footer' => null])\n<div>{{ $title }}</div>\n",
		"resources/views/page.blade.php":                  page,
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
	h = newHarness(t)
	h.readResponse(h.send("initialize", map[string]interface{}{
		"rootUri": symbols.PathToURI(root), "capabilities": map[string]interface{}{}, "processId": nil,
	}))
	h.notify("initialized", map[string]interface{}{})
	time.Sleep(500 * time.Millisecond)
	uri = symbols.PathToURI(filepath.Join(root, "resources", "views", "page.blade.php"))
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "blade", "version": 1, "text": page},
	})
	time.Sleep(200 * time.Millisecond)
	return h, root, uri
}

func TestBladeComponentCompletion(t *testing.T) {
	page := "<x-\n<x-alert \n<x-card \n"
	h, _, uri := componentsWorkspace(t, page)
	defer h.close()

	labels := func(line, char int) []string {
		var items []testCompletionItem
		request(t, h, "textDocument/completion", uri, map[string]interface{}{"line": line, "character": char}, nil, &items)
		var out []string
		for _, it := range items {
			out = append(out, it.Label)
		}
		return out
	}
	if got := strings.Join(labels(0, 3), " "); got != "alert card" {
		t.Fatalf("component names: %q", got)
	}
	if got := strings.Join(labels(1, 9), " "); got != "type alert-title" {
		t.Fatalf("class component attributes: %q", got)
	}
	if got := strings.Join(labels(2, 8), " "); got != "title footer" {
		t.Fatalf("anonymous component attributes: %q", got)
	}
}

func TestBladeComponentDefinitionAndHover(t *testing.T) {
	h, root, uri := componentsWorkspace(t, componentPage)
	defer h.close()

	var locs []testLocation
	request(t, h, "textDocument/definition", uri, posOf(t, componentPage, "alert", 1), nil, &locs)
	classURI := symbols.PathToURI(filepath.Join(root, "app", "View", "Components", "Alert.php"))
	if len(locs) == 0 || locs[0].URI != classURI || locs[0].Range.Start.Line != 6 {
		t.Fatalf("expected class Alert, got %+v", locs)
	}

	locs = nil
	request(t, h, "textDocument/definition", uri, posOf(t, componentPage, "card", 1), nil, &locs)
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "components/card/index.blade.php") {
		t.Fatalf("expected the card view, got %+v", locs)
	}

	var hover struct{ Contents struct{ Value string } }
	request(t, h, "textDocument/hover", uri, posOf(t, componentPage, "alert", 1), nil, &hover)
	if !strings.Contains(hover.Contents.Value, `App\View\Components\Alert`) || !strings.Contains(hover.Contents.Value, "alert-title") {
		t.Fatalf("hover: %q", hover.Contents.Value)
	}
}

func TestBladeUnknownComponent(t *testing.T) {
	h, _, uri := componentsWorkspace(t, componentPage)
	defer h.close()

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
		if strings.Join(last, "\n") == "unknown-component: Component `x-missing` not found" {
			return
		}
	}
	t.Fatalf("expected x-missing reported, last got %q", last)
}
