package lsp

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const laravelComposer = `{"require": {"php": "^8.2", "laravel/framework": "^11.0"}}`

func completionLabelsAt(t *testing.T, h *lspHarness, uri string, line, character int) []string {
	t.Helper()
	resp := h.readResponse(h.send("textDocument/completion", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": character},
	}))
	if resp["error"] != nil {
		t.Fatalf("completion: %v", resp["error"])
	}
	var labels []string
	items, _ := resp["result"].([]interface{})
	if list, ok := resp["result"].(map[string]interface{}); ok {
		items, _ = list["items"].([]interface{})
	}
	for _, raw := range items {
		if label, ok := raw.(map[string]interface{})["label"].(string); ok {
			labels = append(labels, label)
		}
	}
	return labels
}

func waitForCompletion(t *testing.T, h *lspHarness, uri string, line, character int, want func([]string) bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		labels := completionLabelsAt(t, h, uri, line, character)
		if want(labels) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("completion at %d:%d kept answering %v", line, character, labels)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAViewCreatedOutsideTheEditorIsCompleted(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":                     laravelComposer,
		"resources/views/welcome.blade.php": "<h1>Hi</h1>\n",
	}, watchingClient(), nil)
	uri := symbols.PathToURI(filepath.Join(root, "app/Http/Controllers/HomeController.php"))
	text := "<?php\nfunction home() { return view('orders.'); }\n"
	openDocument(h, uri, text)
	character := len("function home() { return view('orders.")
	if labels := completionLabelsAt(t, h, uri, 1, character); slices.Contains(labels, "orders.show") {
		t.Fatalf("the view doesn't exist yet: %v", labels)
	}

	writeProjectFiles(t, root, map[string]string{"resources/views/orders/show.blade.php": "<p>Order</p>\n"})
	notifyWatched(h, root, "resources/views/orders/show.blade.php")
	waitForCompletion(t, h, uri, 1, character, func(labels []string) bool { return slices.Contains(labels, "orders.show") })
}

func TestATranslationSavedInTheEditorIsCompletedWithoutWatchers(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json": laravelComposer,
		"lang/en.json":  "{\n  \"Hello\": \"Hello\"\n}\n",
	}, map[string]interface{}{}, nil)
	uri := symbols.PathToURI(filepath.Join(root, "app/Greeting.php"))
	text := "<?php\nfunction greet() { return __('Go'); }\n"
	openDocument(h, uri, text)
	character := len("function greet() { return __('Go")
	if labels := completionLabelsAt(t, h, uri, 1, character); slices.Contains(labels, "Goodbye") {
		t.Fatalf("the key doesn't exist yet: %v", labels)
	}

	lang := symbols.PathToURI(filepath.Join(root, "lang/en.json"))
	saved := "{\n  \"Hello\": \"Hello\",\n  \"Goodbye\": \"Goodbye\"\n}\n"
	writeProjectFiles(t, root, map[string]string{"lang/en.json": saved})
	h.notify("textDocument/didOpen", map[string]interface{}{"textDocument": map[string]interface{}{
		"uri": lang, "languageId": "json", "version": 1, "text": saved,
	}})
	h.notify("textDocument/didSave", map[string]interface{}{"textDocument": map[string]interface{}{"uri": lang}})
	waitForCompletion(t, h, uri, 1, character, func(labels []string) bool { return slices.Contains(labels, "Goodbye") })
}
