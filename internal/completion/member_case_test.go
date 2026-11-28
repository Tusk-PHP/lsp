package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestCompletionAfterAMethodCalledInAnotherCase(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Room.php", "<?php\nnamespace App;\nclass Size { public function width(): int { return 1; } }\nclass Room { public function area(): Size { return new Size(); } }\n")
	p := NewProvider(idx, nil, "")
	source := "<?php\nnamespace App;\nfunction f(Room $room) {\n    $room->AREA()->\n}\n"
	found := false
	for _, item := range p.GetCompletions("file:///app/use.php", source, protocol.Position{Line: 3, Character: 19}) {
		found = found || item.Label == "width"
	}
	if !found {
		t.Error("the chain after AREA() is typed by area()'s return type")
	}
}
