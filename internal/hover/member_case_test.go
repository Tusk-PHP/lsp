package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnAMethodCalledInAnotherCase(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Room.php", "<?php\nnamespace App;\nclass Room {\n    /** The floor area. */\n    public function area(): int { return 1; }\n}\n")
	source := "<?php\nnamespace App;\nfunction f(Room $room) {\n    $room->AREA();\n}\n"
	h := NewProvider(idx, nil, "none").GetHover("file:///app/use.php", source, protocol.Position{Line: 3, Character: 13})
	if h == nil || !strings.Contains(h.Contents.Value, "Room::area") || !strings.Contains(h.Contents.Value, "The floor area.") {
		t.Fatalf("hover shows area() for a call written AREA(), got %+v", h)
	}
}
