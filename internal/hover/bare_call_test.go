package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnABareCallShowsOnlyAFunction(t *testing.T) {
	room := "<?php\nnamespace Other;\nclass Room {\n    public function area(): float { return 1.0; }\n    public function measure(): float { return area() + perimeter(); }\n}\n"
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///Room.php", room)
	idx.IndexFile("file:///helpers.php", "<?php\nfunction perimeter(): float { return 0.0; }\n")
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	at := func(needle string) protocol.Position {
		i := strings.Index(room, needle)
		return protocol.Position{Line: strings.Count(room[:i], "\n"), Character: i - strings.LastIndex(room[:i], "\n") - 1}
	}
	if h := p.GetHover("file:///Room.php", room, at("area() +")); h != nil {
		t.Errorf("a bare call inside the class doesn't show its method: %+v", h.Contents.Value)
	}
	h := p.GetHover("file:///Room.php", room, at("perimeter()"))
	if h == nil || !strings.Contains(h.Contents.Value, "function perimeter") {
		t.Errorf("a bare call shows the function PHP calls, got %+v", h)
	}
}
