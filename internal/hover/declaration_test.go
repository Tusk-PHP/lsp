package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnADeclaredNameShowsThatDeclaration(t *testing.T) {
	src := `<?php
namespace App;
interface Sized { /** How many there are. */ public function size(): int; }
class Bag implements Sized, \Countable {
    public function count(): int { return 0; }
    public function reset(int $to): void {}
    public function size(): int { return 0; }
    public function weigh(): float { return 0.0; }
}
function filter(array $items): array { return $items; }
`
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///Bag.php", src)
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	hoverOn := func(needle string) string {
		t.Helper()
		i := strings.Index(src, needle) + len("function ")
		pos := protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1}
		h := p.GetHover("file:///Bag.php", src, pos)
		if h == nil {
			t.Fatalf("no hover on %s", needle)
		}
		return h.Contents.Value
	}
	cases := []struct{ needle, title, signature string }{
		{"function count()", `**App\Bag::count**`, "public function count(): int"},
		{"function reset(", `**App\Bag::reset**`, "public function reset(int $to): void"},
		{"function weigh(", `**App\Bag::weigh**`, "public function weigh(): float"},
		{"function size(): int { return", `**App\Bag::size**`, "Implements `App\\Sized::size`"},
		{"function filter(", `**App\filter**`, "function filter(array $items): array"},
	}
	for _, c := range cases {
		got := hoverOn(c.needle)
		if !strings.Contains(got, c.title) || !strings.Contains(got, c.signature) {
			t.Errorf("hover on the name in %q shows that declaration (%s, %s), got:\n%s", c.needle, c.title, c.signature, got)
		}
	}
}

func TestHoverOnAGlobalDeclaration(t *testing.T) {
	src := "<?php\nfunction weigh(array $items): float { return 0.0; }\n"
	pos := protocol.Position{Line: 1, Character: len("function we")}
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")

	if h := p.GetHover("file:///weigh.php", src, pos); h != nil {
		t.Errorf("a declaration the index doesn't have yet shows nothing, got:\n%s", h.Contents.Value)
	}

	idx.IndexFile("file:///weigh.php", src)
	h := p.GetHover("file:///weigh.php", src, pos)
	if h == nil || !strings.Contains(h.Contents.Value, "**weigh**") || !strings.Contains(h.Contents.Value, "function weigh(array $items): float") {
		t.Errorf("hover on a global function's declaration shows it, got %+v", h)
	}
}
