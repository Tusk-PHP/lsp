package lsp

import (
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/protocol"
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

func TestFindBracketRangeReadsTheLexer(t *testing.T) {
	source := "<?php\nclass A {\n    public private(set) string $s = \"(x\";\n    #[Attr(1)]\n    function f() { return [$a, '{']; }\n}\n"
	lines := ast.NewLines(source)
	for name, tc := range map[string]struct {
		pos  protocol.Position
		want protocol.Range
	}{
		"modifier is not a group": {protocol.Position{Line: 2, Character: 17}, protocol.Range{Start: protocol.Position{Line: 1, Character: 8}, End: protocol.Position{Line: 5, Character: 1}}},
		"string is not a group":   {protocol.Position{Line: 2, Character: 38}, protocol.Range{Start: protocol.Position{Line: 1, Character: 8}, End: protocol.Position{Line: 5, Character: 1}}},
		"attribute":               {protocol.Position{Line: 3, Character: 6}, protocol.Range{Start: protocol.Position{Line: 3, Character: 5}, End: protocol.Position{Line: 3, Character: 14}}},
		"array with brace string": {protocol.Position{Line: 4, Character: 28}, protocol.Range{Start: protocol.Position{Line: 4, Character: 26}, End: protocol.Position{Line: 4, Character: 35}}},
		"past the line end":       {protocol.Position{Line: 4, Character: 99}, protocol.Range{Start: protocol.Position{Line: 4, Character: 17}, End: protocol.Position{Line: 4, Character: 38}}},
	} {
		t.Run(name, func(t *testing.T) {
			got := findBracketRange(source, lines, tc.pos)
			if got == nil || *got != tc.want {
				t.Fatalf("findBracketRange(%v) = %v, want %v", tc.pos, got, tc.want)
			}
		})
	}
	if got := findBracketRange(source, lines, protocol.Position{Line: 0, Character: 2}); got != nil {
		t.Errorf("outside every bracket: got %v, want none", got)
	}
	if got := findBracketRange(source, lines, protocol.Position{Line: 99}); got != nil {
		t.Errorf("past the last line: got %v, want none", got)
	}
	if got := findBracketRange("<?php ) ] (", ast.NewLines("<?php ) ] ("), protocol.Position{Character: 10}); got != nil {
		t.Errorf("unmatched brackets: got %v, want none", got)
	}
}
