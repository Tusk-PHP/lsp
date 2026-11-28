package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnBuiltinInterfaceMethods(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	src := "<?php\nclass Bag implements \\Countable {\n    public function count(): int { return 0; }\n}\nfunction f(\\Countable $items, Bag $bag): void {\n    $items->count();\n    $bag->count();\n}\n"
	idx.IndexFile("file:///a.php", src)
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	hoverAt := func(needle string) string {
		t.Helper()
		i := strings.Index(src, needle) + strings.Index(needle, "count")
		pos := protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1}
		h := p.GetHover("file:///a.php", src, pos)
		if h == nil {
			t.Fatalf("no hover on %s", needle)
		}
		return h.Contents.Value
	}
	if got := hoverAt("$items->count()"); !strings.Contains(got, "**Countable::count**") || !strings.Contains(got, "abstract public function count(): int") {
		t.Errorf("hover on Countable::count shows the interface's method: %s", got)
	}
	if got := hoverAt("$bag->count()"); !strings.Contains(got, "**Bag::count**") || !strings.Contains(got, "Implements `Countable::count`") {
		t.Errorf("hover on Bag::count names the Countable::count it implements: %s", got)
	}
}
