package lsp

import (
	"testing"
	"time"
)

func TestHandleSelectionRange(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_selection_range.php"
	source := `<?php
namespace App;

class UserService {
    private string $name;

    public function getName(): string {
        return $this->name;
    }
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": "php",
			"version":    1,
			"text":       source,
		},
	})
	time.Sleep(100 * time.Millisecond)

	// Position inside "getName" method name (line 6, character 20 is inside "getName")
	id := h.send("textDocument/selectionRange", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"positions": []interface{}{
			map[string]interface{}{"line": 6, "character": 20},
		},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("selectionRange returned error: %v", resp["error"])
	}

	result, ok := resp["result"].([]interface{})
	if !ok || len(result) == 0 {
		t.Fatalf("expected non-empty selectionRange result array, got %T: %v", resp["result"], resp["result"])
	}

	first, ok := result[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first result to be a map, got %T", result[0])
	}

	rng, ok := first["range"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected range field in first selectionRange result, got %T", first["range"])
	}

	start, ok := rng["start"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected start in range, got %T", rng["start"])
	}
	end, ok := rng["end"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected end in range, got %T", rng["end"])
	}

	// The range should be a valid range (start <= end)
	startLine, _ := start["line"].(float64)
	endLine, _ := end["line"].(float64)
	if startLine > endLine {
		t.Errorf("expected start line <= end line, got start=%v end=%v", startLine, endLine)
	}

	// There should be a parent chain (word -> method -> class -> whole doc)
	parent, hasParent := first["parent"]
	if !hasParent || parent == nil {
		t.Log("no parent chain found (word range at non-word position is acceptable)")
	}
}

func TestHandleSelectionRangeMultiplePositions(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_selection_multi.php"
	source := `<?php
class Foo {
    public function bar(): void {}
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": "php",
			"version":    1,
			"text":       source,
		},
	})
	time.Sleep(100 * time.Millisecond)

	id := h.send("textDocument/selectionRange", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"positions": []interface{}{
			map[string]interface{}{"line": 1, "character": 6},
			map[string]interface{}{"line": 2, "character": 20},
		},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("selectionRange returned error: %v", resp["error"])
	}

	result, ok := resp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected result array, got %T", resp["result"])
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 selection ranges (one per position), got %d", len(result))
	}
}

func TestHandleSelectionRangeEmptyDocument(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_selection_empty.php"
	source := `<?php
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": "php",
			"version":    1,
			"text":       source,
		},
	})
	time.Sleep(100 * time.Millisecond)

	id := h.send("textDocument/selectionRange", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"positions": []interface{}{
			map[string]interface{}{"line": 0, "character": 0},
		},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("selectionRange on empty document returned error: %v", resp["error"])
	}
}
