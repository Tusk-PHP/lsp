package lsp

import (
	"strings"
	"testing"
	"time"
)

func TestHandleCodeActionGenerateGetterSetter(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_getter_setter.php"
	source := `<?php
namespace App;

class User {
    private string $name;
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
			"start": map[string]interface{}{"line": 4, "character": 4},
			"end":   map[string]interface{}{"line": 4, "character": 24},
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

	var combined map[string]interface{}
	for _, raw := range result {
		action, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected code action object, got %T", raw)
		}
		if action["title"] == "Generate getter and setter for $name" {
			combined = action
			break
		}
	}

	if combined == nil {
		t.Fatalf("expected getter/setter refactor in %#v", result)
	}
	if kind, _ := combined["kind"].(string); kind != "refactor" {
		t.Fatalf("expected refactor kind, got %q", kind)
	}

	edit := combined["edit"].(map[string]interface{})
	changes := edit["changes"].(map[string]interface{})[uri].([]interface{})
	if len(changes) != 1 {
		t.Fatalf("expected one edit, got %#v", changes)
	}

	newText, _ := changes[0].(map[string]interface{})["newText"].(string)
	if !strings.Contains(newText, "public function getName(): string") {
		t.Fatalf("combined edit missing getter:\n%s", newText)
	}
	if !strings.Contains(newText, "public function setName(string $name): void") {
		t.Fatalf("combined edit missing setter:\n%s", newText)
	}
}
