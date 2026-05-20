package lsp

import (
	"testing"
	"time"
)

func TestHandleCodeActionUnusedImportQuickFixAndOrganizeImportsImportSlice(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_import_actions.php"
	source := `<?php
use const App\Config\MAX_RETRIES;
use App\Models\Post;
use function App\Helpers\formatName;
use App\Models\User;

$user = new User();
$name = formatName('Ada');
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	diagnostics := []interface{}{
		map[string]interface{}{
			"range": map[string]interface{}{
				"start": map[string]interface{}{"line": 1, "character": 0},
				"end":   map[string]interface{}{"line": 1, "character": 32},
			},
			"message":  "Unused import 'App\\Config\\MAX_RETRIES'",
			"severity": 4,
			"source":   "tusk-php",
			"code":     "unused-import",
		},
		map[string]interface{}{
			"range": map[string]interface{}{
				"start": map[string]interface{}{"line": 2, "character": 0},
				"end":   map[string]interface{}{"line": 2, "character": 20},
			},
			"message":  "Unused import 'App\\Models\\Post'",
			"severity": 4,
			"source":   "tusk-php",
			"code":     "unused-import",
		},
	}

	id := h.send("textDocument/codeAction", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 2, "character": 0},
			"end":   map[string]interface{}{"line": 2, "character": 20},
		},
		"context": map[string]interface{}{"diagnostics": diagnostics},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("codeAction returned error: %v", resp["error"])
	}

	result, ok := resp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected codeAction array, got %T", resp["result"])
	}

	var quickfixAction map[string]interface{}
	var organizeAction map[string]interface{}
	for _, raw := range result {
		action, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected code action object, got %T", raw)
		}
		switch action["kind"] {
		case "quickfix":
			if action["title"] == "Remove unused import" {
				quickfixAction = action
			}
		case "source.organizeImports":
			organizeAction = action
		}
	}

	if quickfixAction == nil {
		t.Fatalf("expected remove unused import quickfix in %#v", result)
	}
	if organizeAction == nil {
		t.Fatalf("expected organize imports action in %#v", result)
	}

	qfChanges := quickfixAction["edit"].(map[string]interface{})["changes"].(map[string]interface{})[uri].([]interface{})
	if len(qfChanges) != 1 {
		t.Fatalf("expected one quickfix edit, got %#v", qfChanges)
	}
	qfEdit := qfChanges[0].(map[string]interface{})
	if newText, _ := qfEdit["newText"].(string); newText != "" {
		t.Fatalf("expected deletion quickfix, got %q", newText)
	}

	orgChanges := organizeAction["edit"].(map[string]interface{})["changes"].(map[string]interface{})[uri].([]interface{})
	if len(orgChanges) != 1 {
		t.Fatalf("expected one organize edit, got %#v", orgChanges)
	}
	orgEdit := orgChanges[0].(map[string]interface{})
	if newText, _ := orgEdit["newText"].(string); newText != "use App\\Models\\User;\n\nuse function App\\Helpers\\formatName;" {
		t.Fatalf("unexpected organize imports replacement %q", newText)
	}
}
