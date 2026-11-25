package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnAnAnonymousClassMethodShowsThatMethod(t *testing.T) {
	src := `<?php
namespace App;
class Room {
    /** Room's area. */
    public function area(): float { return 1.0; }
    public function make() {
        return new class {
            /** The anonymous one. */
            public function area(): int { return 2; }
        };
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Room.php", src)
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	i := strings.Index(src, "area(): int")
	pos := protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1}
	h := p.GetHover("file:///Room.php", src, pos)
	if h == nil || !strings.Contains(h.Contents.Value, "The anonymous one") || strings.Contains(h.Contents.Value, "Room's area") {
		t.Errorf("hover on the anonymous class's area() shows it, not Room::area: %+v", h)
	}
}
