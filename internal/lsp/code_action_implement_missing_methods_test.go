package lsp

import (
	"testing"
	"time"
)

func TestHandleCodeActionImplementMissingMethods(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	ifaceURI := "file:///tmp/repository.php"
	ifaceSource := `<?php
namespace App;

interface RepositoryInterface {
    public function save(string $id): void;
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": ifaceURI, "languageId": "php", "version": 1, "text": ifaceSource,
		},
	})

	uri := "file:///tmp/test_implement_missing.php"
	source := `<?php
namespace App;

class UserRepository implements RepositoryInterface {
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/codeAction", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 3, "character": 0},
			"end":   map[string]interface{}{"line": 3, "character": 48},
		},
		"context": map[string]interface{}{
			"diagnostics": []interface{}{},
			"only":        []string{"refactor"},
		},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("codeAction returned error: %v", resp["error"])
	}

	result, ok := resp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected codeAction array, got %T", resp["result"])
	}

	var action map[string]interface{}
	for _, raw := range result {
		candidate, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected code action object, got %T", raw)
		}
		if title, _ := candidate["title"].(string); title == "Implement missing methods" {
			action = candidate
			break
		}
	}
	if action == nil {
		t.Fatalf("expected implement missing methods action in %#v", result)
	}

	edit, ok := action["edit"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workspace edit, got %#v", action["edit"])
	}
	changes, ok := edit["changes"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected changes map, got %#v", edit["changes"])
	}
	fileChanges, ok := changes[uri].([]interface{})
	if !ok || len(fileChanges) != 1 {
		t.Fatalf("expected one file edit, got %#v", changes[uri])
	}
	firstEdit, ok := fileChanges[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected edit object, got %T", fileChanges[0])
	}
	if newText, _ := firstEdit["newText"].(string); newText == "" {
		t.Fatal("expected non-empty generated method text")
	}
}
