package lsp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const authLSPProvider = `<?php
namespace App\Providers;

use Illuminate\Support\Facades\Gate;

class AuthServiceProvider
{
    public function boot(): void
    {
        Gate::define('view-dashboard', fn ($user) => true);
    }
}
`

const authLSPGateContract = `<?php
namespace Illuminate\Contracts\Auth\Access;

interface Gate
{
    public function check($abilities, $arguments = []);
    public function denies($ability, $arguments = []);
    public function any($abilities, $arguments = []);
}
`

func authWorkspace(t *testing.T, view string) (h *lspHarness, viewURI, providerPath string) {
	t.Helper()
	root := t.TempDir()
	viewPath := filepath.Join(root, "resources", "views", "home.blade.php")
	providerPath = filepath.Join(root, "app", "Providers", "AuthServiceProvider.php")
	files := map[string]string{
		filepath.Join(root, "composer.json"): `{"require": {"laravel/framework": "^11.0"}, "autoload": {"psr-4": {"App\\": "app/"}}}`,
		providerPath:                         authLSPProvider,
		filepath.Join(root, "app", "Policies", "PostPolicy.php"):            "<?php\nnamespace App\\Policies;\n\nclass PostPolicy\n{\n    public function publish($user, $post) {}\n}\n",
		filepath.Join(root, "stubs", "Illuminate", "Contracts", "Gate.php"): authLSPGateContract,
		viewPath: view,
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
	return h, viewURI, providerPath
}

type authCompletionItem struct {
	Label    string
	TextEdit *struct {
		Range   testRange
		NewText string
	}
}

func completionLabels(t *testing.T, h *lspHarness, uri string, at map[string]interface{}) ([]string, []authCompletionItem) {
	t.Helper()
	var items []authCompletionItem
	request(t, h, "textDocument/completion", uri, at, nil, &items)
	labels := make([]string, 0, len(items))
	for _, it := range items {
		labels = append(labels, it.Label)
	}
	return labels, items
}

func TestBladeGateDirectivesCompleteAbilities(t *testing.T) {
	view := "@can('')\n<p>x</p>\n@elsecannot('')\n@endcan\n@canany(['view-dashboard', 'pu'])\n@endcanany\n"
	h, viewURI, providerPath := authWorkspace(t, view)
	defer h.close()

	for _, at := range []map[string]interface{}{
		posOf(t, view, "@can('", len("@can('")),
		posOf(t, view, "@elsecannot('", len("@elsecannot('")),
	} {
		labels, _ := completionLabels(t, h, viewURI, at)
		if !slices.Contains(labels, "view-dashboard") || !slices.Contains(labels, "publish") {
			t.Errorf("at %v: Gate and policy abilities are offered: %v", at, labels)
		}
	}
	labels, items := completionLabels(t, h, viewURI, posOf(t, view, "'pu", len("'pu")))
	if !slices.Equal(labels, []string{"publish"}) {
		t.Fatalf("@canany offers the abilities starting with what is typed: %v", labels)
	}
	if e := items[0].TextEdit; e == nil || e.Range.Start.Line != 4 || e.Range.Start.Character != 28 || e.Range.End.Character != 30 {
		t.Errorf("the edit is placed in the view, over what is typed: %+v", items[0].TextEdit)
	}

	edited := strings.Replace(authLSPProvider, "view-dashboard", "manage-team", 1)
	providerURI := "file://" + filepath.ToSlash(providerPath)
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": providerURI, "languageId": "php", "version": 1, "text": authLSPProvider},
	})
	h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument":   map[string]interface{}{"uri": providerURI, "version": 2},
		"contentChanges": []map[string]interface{}{{"text": edited}},
	})
	time.Sleep(300 * time.Millisecond)
	labels, _ = completionLabels(t, h, viewURI, posOf(t, view, "@can('", len("@can('")))
	if !slices.Contains(labels, "manage-team") || slices.Contains(labels, "view-dashboard") {
		t.Errorf("an edited provider changes the abilities: %v", labels)
	}
}

func TestBladeGateDirectiveGoesToTheAbility(t *testing.T) {
	view := "@can('view-dashboard')\n@endcan\n"
	h, viewURI, providerPath := authWorkspace(t, view)
	defer h.close()

	var loc testLocation
	request(t, h, "textDocument/definition", viewURI, posOf(t, view, "view-dashboard", 3), nil, &loc)
	if !strings.HasSuffix(loc.URI, filepath.ToSlash(providerPath)) || loc.Range.Start.Line != 9 {
		t.Errorf("@can goes to Gate::define: %+v", loc)
	}
}
