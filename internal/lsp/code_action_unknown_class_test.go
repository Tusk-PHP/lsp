package lsp

import (
	"testing"
	"time"
)

func TestHandleCodeActionUnknownClassQuickFix(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	vendorURI := "file:///tmp/vendor_logger.php"
	vendorSource := `<?php
namespace Monolog;
class Logger {}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": vendorURI, "languageId": "php", "version": 1, "text": vendorSource,
		},
	})

	uri := "file:///tmp/test_unknown_class_action.php"
	source := `<?php
namespace App;

class Demo {
    public function run(): void {
        new Logger();
    }
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
			"start": map[string]interface{}{"line": 5, "character": 12},
			"end":   map[string]interface{}{"line": 5, "character": 18},
		},
		"context": map[string]interface{}{
			"diagnostics": []interface{}{
				map[string]interface{}{
					"range": map[string]interface{}{
						"start": map[string]interface{}{"line": 5, "character": 12},
						"end":   map[string]interface{}{"line": 5, "character": 18},
					},
					"message":  "Unknown class 'Logger'",
					"severity": 2,
					"source":   "tusk-php",
					"code":     "unknown-class",
				},
			},
			"only": []string{"quickfix"},
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
	if len(result) != 1 {
		t.Fatalf("expected one quickfix action, got %#v", result)
	}

	action, ok := result[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected code action object, got %T", result[0])
	}
	if title, _ := action["title"].(string); title != "Import Monolog\\Logger" {
		t.Fatalf("unexpected title %q", title)
	}
	if kind, _ := action["kind"].(string); kind != "quickfix" {
		t.Fatalf("unexpected kind %q", kind)
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
	if newText, _ := firstEdit["newText"].(string); newText != "use Monolog\\Logger;\n" {
		t.Fatalf("unexpected newText %q", newText)
	}
}
