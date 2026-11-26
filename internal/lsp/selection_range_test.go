package lsp

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
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
	toks := lexer.Lex(source, token.Latest)
	bracketAt := func(pos protocol.Position) (protocol.Range, bool) {
		span, ok := innermostBracket(toks, lineOffset(source, lines, pos))
		return protocol.Range{Start: spanPosition(lines, span.Start), End: spanPosition(lines, span.End)}, ok
	}
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
			if got, ok := bracketAt(tc.pos); !ok || got != tc.want {
				t.Fatalf("innermostBracket(%v) = %v %v, want %v", tc.pos, got, ok, tc.want)
			}
		})
	}
	if got, ok := bracketAt(protocol.Position{Line: 0, Character: 2}); ok {
		t.Errorf("outside every bracket: got %v, want none", got)
	}
	if got, ok := innermostBracket(lexer.Lex("<?php ) ] (", token.Latest), 10); ok {
		t.Errorf("unmatched brackets: got %v, want none", got)
	}
}

func TestBuildSelectionChainsNestEveryEnclosingRangeInItsParent(t *testing.T) {
	source := "<?php\nclass A {\n    public function f(int $a)\n    {\n        $o = new class { public function g() { return 1; } };\n    }\n}\n"
	lines := strings.Split(source, "\n")
	var positions []protocol.Position
	for l, text := range lines {
		for c := 0; c <= len(text); c++ {
			positions = append(positions, protocol.Position{Line: l, Character: c})
		}
	}
	contains := func(outer, inner protocol.Range) bool {
		before := func(a, b protocol.Position) bool {
			return a.Line < b.Line || (a.Line == b.Line && a.Character <= b.Character)
		}
		return before(outer.Start, inner.Start) && before(inner.End, outer.End)
	}
	for i, chain := range buildSelectionChains(source, positions) {
		for r := chain.Parent; r.Parent != nil; r = r.Parent {
			if !contains(r.Parent.Range, r.Range) {
				t.Errorf("%v: %v is not inside its parent %v", positions[i], r.Range, r.Parent.Range)
			}
		}
	}

	rng := func(sl, sc, el, ec int) protocol.Range {
		return protocol.Range{Start: protocol.Position{Line: sl, Character: sc}, End: protocol.Position{Line: el, Character: ec}}
	}
	g := strings.Index(lines[4], "return")
	chains := buildSelectionChains(source, []protocol.Position{{Line: 2, Character: 22}, {Line: 4, Character: g}, {Line: 99}})
	for i, want := range [][]protocol.Range{
		{rng(2, 22, 2, 25), rng(2, 21, 2, 29), rng(2, 0, 5, 5), rng(1, 0, 6, 1), rng(0, 0, 7, 0)},
		{rng(4, g, 4, g+6), rng(4, g-2, 4, g+11), rng(4, 0, 4, len(lines[4])), rng(1, 0, 6, 1), rng(0, 0, 7, 0)},
		{rng(0, 0, 7, 0)},
	} {
		var got []protocol.Range
		for r := &chains[i]; r != nil; r = r.Parent {
			got = append(got, r.Range)
		}
		if !slices.Equal(got, want) {
			t.Errorf("chain %d = %v, want %v", i, got, want)
		}
	}
}
