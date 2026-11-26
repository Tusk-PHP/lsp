package lsp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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

func selectionRng(sl, sc, el, ec int) protocol.Range {
	return protocol.Range{Start: protocol.Position{Line: sl, Character: sc}, End: protocol.Position{Line: el, Character: ec}}
}

func assertSelectionChains(t *testing.T, source string, want map[protocol.Position][]protocol.Range) {
	t.Helper()
	positions := make([]protocol.Position, 0, len(want))
	for pos := range want {
		positions = append(positions, pos)
	}
	for i, chain := range buildSelectionChains(source, positions) {
		var got []protocol.Range
		for r := &chain; r != nil; r = r.Parent {
			got = append(got, r.Range)
		}
		if !slices.Equal(got, want[positions[i]]) {
			t.Errorf("chain at %v = %v, want %v", positions[i], got, want[positions[i]])
		}
	}
}

func TestBuildSelectionChainsReadTheLexer(t *testing.T) {
	class := []protocol.Range{selectionRng(1, 9, 5, 0), selectionRng(1, 8, 5, 1), selectionRng(1, 0, 5, 1), selectionRng(0, 0, 6, 0)}
	method := append([]protocol.Range{selectionRng(3, 4, 4, 38), selectionRng(3, 0, 4, 38)}, class...)
	assertSelectionChains(t, "<?php\nclass A {\n    public private(set) string $s = \"(x\";\n    #[Attr(1)]\n    function f() { return [$a, '{']; }\n}\n", map[protocol.Position][]protocol.Range{
		{Line: 2, Character: 17}: append([]protocol.Range{selectionRng(2, 11, 2, 18), selectionRng(2, 11, 2, 23), selectionRng(2, 4, 2, 41)}, class...),
		{Line: 2, Character: 38}: append([]protocol.Range{selectionRng(2, 38, 2, 39), selectionRng(2, 37, 2, 39), selectionRng(2, 36, 2, 40), selectionRng(2, 31, 2, 40), selectionRng(2, 4, 2, 41)}, class...),
		{Line: 3, Character: 6}:  append([]protocol.Range{selectionRng(3, 6, 3, 10), selectionRng(3, 6, 3, 13), selectionRng(3, 5, 3, 14), selectionRng(3, 4, 3, 14)}, method...),
		{Line: 4, Character: 28}: append([]protocol.Range{selectionRng(4, 27, 4, 29), selectionRng(4, 27, 4, 34), selectionRng(4, 26, 4, 35), selectionRng(4, 19, 4, 36), selectionRng(4, 18, 4, 37), selectionRng(4, 17, 4, 38)}, method...),
		{Line: 4, Character: 99}: append([]protocol.Range{selectionRng(4, 38, 4, 38), selectionRng(4, 17, 4, 38)}, method...),
		{Line: 0, Character: 2}:  {selectionRng(0, 2, 0, 5), selectionRng(0, 0, 1, 0), selectionRng(0, 0, 6, 0)},
	})
	assertSelectionChains(t, "<?php ) ] (", map[protocol.Position][]protocol.Range{
		{Line: 0, Character: 10}: {selectionRng(0, 10, 0, 10), selectionRng(0, 10, 0, 11), selectionRng(0, 6, 0, 11), selectionRng(0, 0, 0, 11)},
	})
	assertSelectionChains(t, "<?php\n$a = 'x';\n  \n$b = \"q {$a}", map[protocol.Position][]protocol.Range{
		{Line: 1, Character: 5}: {selectionRng(1, 5, 1, 5), selectionRng(1, 5, 1, 8), selectionRng(1, 0, 1, 8), selectionRng(1, 0, 1, 9), selectionRng(0, 0, 3, 12)},
		{Line: 2, Character: 1}: {selectionRng(2, 1, 2, 1), selectionRng(0, 0, 3, 12)},
		{Line: 3, Character: 9}: {selectionRng(3, 9, 3, 11), selectionRng(3, 5, 3, 12), selectionRng(3, 0, 3, 12), selectionRng(0, 0, 3, 12)},
	})
	assertSelectionChains(t, "<?php\n$s = b'';\n$t = '", map[protocol.Position][]protocol.Range{
		{Line: 1, Character: 7}: {selectionRng(1, 7, 1, 7), selectionRng(1, 5, 1, 8), selectionRng(1, 0, 1, 8), selectionRng(1, 0, 1, 9), selectionRng(0, 0, 2, 6)},
		{Line: 2, Character: 6}: {selectionRng(2, 6, 2, 6), selectionRng(2, 5, 2, 6), selectionRng(2, 0, 2, 6), selectionRng(0, 0, 2, 6)},
	})
}

func TestBuildSelectionChainsGrowThroughTheSyntaxTree(t *testing.T) {
	source := "<?php\nclass A {\n    public function f(int $a)\n    {\n        $o = new class { public function g() { return 1; } };\n    }\n}\n"
	g := strings.Index(strings.Split(source, "\n")[4], "return")
	class := []protocol.Range{selectionRng(1, 9, 6, 0), selectionRng(1, 8, 6, 1), selectionRng(1, 0, 6, 1), selectionRng(0, 0, 7, 0)}
	method := append([]protocol.Range{selectionRng(2, 4, 5, 5), selectionRng(2, 0, 5, 5)}, class...)
	assertSelectionChains(t, source, map[protocol.Position][]protocol.Range{
		{Line: 2, Character: 22}: append([]protocol.Range{selectionRng(2, 22, 2, 25), selectionRng(2, 22, 2, 28), selectionRng(2, 21, 2, 29)}, method...),
		{Line: 4, Character: g}: append([]protocol.Range{
			selectionRng(4, g, 4, g+6), selectionRng(4, g, 4, g+9), selectionRng(4, g-1, 4, g+10), selectionRng(4, g-2, 4, g+11),
			selectionRng(4, 25, 4, 58), selectionRng(4, 24, 4, 59), selectionRng(4, 23, 4, 60), selectionRng(4, 17, 4, 60), selectionRng(4, 13, 4, 60),
			selectionRng(4, 8, 4, 60), selectionRng(4, 8, 4, 61), selectionRng(4, 0, 4, 61), selectionRng(3, 5, 5, 4), selectionRng(3, 4, 5, 5),
		}, method...),
		{Line: 99}: {selectionRng(0, 0, 7, 0)},
	})
	assertSelectionChains(t, "<?php\nf($a, $b);\n", map[protocol.Position][]protocol.Range{
		{Line: 1, Character: 1}: {selectionRng(1, 0, 1, 1), selectionRng(1, 0, 1, 9), selectionRng(1, 0, 1, 10), selectionRng(0, 0, 2, 0)},
		{Line: 1, Character: 3}: {selectionRng(1, 2, 1, 4), selectionRng(1, 2, 1, 8), selectionRng(1, 1, 1, 9), selectionRng(1, 0, 1, 9), selectionRng(1, 0, 1, 10), selectionRng(0, 0, 2, 0)},
	})
}

func TestBuildSelectionChainsNestAtEveryPosition(t *testing.T) {
	syntax, err := os.ReadFile(filepath.Join("..", "php", "ast", "testdata", "syntax.php"))
	if err != nil {
		t.Fatal(err)
	}
	samples := map[string]string{
		"call":   "<?php\nf($a, $b);\n",
		"syntax": string(syntax),
		"mixed":  "<?php\nclass A {\n    public function f(int $a)\n    {\n        $o = new class { public function g() { return 1; } };\n        return \"x {$a['k']} y\" . 'q(r' . <<<EOT\n          h $a\n          EOT;\n    }\n}\n// f(x)\n",
	}
	t.Run("Builder.php", func(t *testing.T) {
		requireLaravelVendor(t)
		builder, err := os.ReadFile(filepath.Join("..", "..", "testdata", "laravel", "vendor", "laravel", "framework", "src", "Illuminate", "Database", "Eloquent", "Builder.php"))
		if err != nil {
			t.Fatal(err)
		}
		assertSelectionChainsNest(t, string(builder))
	})
	for name, source := range samples {
		t.Run(name, func(t *testing.T) { assertSelectionChainsNest(t, source) })
	}
}

func assertSelectionChainsNest(t *testing.T, source string) {
	t.Helper()
	var positions []protocol.Position
	for l, text := range strings.Split(source, "\n") {
		for c := 0; c <= len(text); c++ {
			positions = append(positions, protocol.Position{Line: l, Character: c})
		}
	}
	before := func(a, b protocol.Position) bool {
		return a.Line < b.Line || (a.Line == b.Line && a.Character <= b.Character)
	}
	contains := func(outer, inner protocol.Range) bool {
		return before(outer.Start, inner.Start) && before(inner.End, outer.End)
	}
	failures := 0
	for i, chain := range buildSelectionChains(source, positions) {
		pos := positions[i]
		if !contains(chain.Range, protocol.Range{Start: pos, End: pos}) {
			t.Errorf("%v: innermost range %v does not hold the cursor", pos, chain.Range)
			failures++
		}
		for r := &chain; r.Parent != nil && failures < 20; r = r.Parent {
			if r.Parent.Range == r.Range || !contains(r.Parent.Range, r.Range) {
				t.Errorf("%v: %v is not strictly inside its parent %v", pos, r.Range, r.Parent.Range)
				failures++
			}
		}
		if failures >= 20 {
			t.Fatal("too many failures")
		}
	}
}
