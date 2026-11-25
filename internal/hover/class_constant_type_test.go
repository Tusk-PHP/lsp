package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnACallWhoseReturnTypeNamesAnArrayConstant(t *testing.T) {
	source := `<?php
namespace App;

class Level {
    public const NAMES = [
        'DEBUG',
        'INFO',
    ];
}

class Record {
    /** @return array{level_name: value-of<Level::NAMES>} */
    public function toArray(): array { return []; }
}

function run(Record $record): void {
    $record->toArray();
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Record.php", source)
	p := NewProvider(idx, nil, "none")
	lines := strings.Split(source, "\n")
	line := len(lines) - 3
	pos := protocol.Position{Line: line, Character: strings.Index(lines[line], "toArray") + 1}

	h := p.GetHover("file:///app/Record.php", source, pos)
	if h == nil || !strings.Contains(h.Contents.Value, "toArray") {
		t.Fatalf("hover on the call shows the method, got %+v", h)
	}
}
