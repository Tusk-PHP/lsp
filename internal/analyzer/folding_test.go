package analyzer

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func TestGetFoldingRanges(t *testing.T) {
	idx := symbols.NewIndex()
	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "", ""))
	source := `<?php
namespace App;

class Example {
    public function build(): array {
        $items = [
            'a' => 1,
            'b' => 2,
        ];
    }
}
`

	ranges := a.GetFoldingRanges("file:///test.php", source)
	assertProtocolFoldRange(t, ranges, 1, 10, "")
	assertProtocolFoldRange(t, ranges, 3, 9, "")
	assertProtocolFoldRange(t, ranges, 4, 8, "")
	assertProtocolFoldRange(t, ranges, 5, 7, protocol.FoldingRangeKindRegion)
}

func assertProtocolFoldRange(t *testing.T, ranges []protocol.FoldingRange, startLine, endLine int, kind protocol.FoldingRangeKind) {
	t.Helper()
	for _, got := range ranges {
		if got.StartLine == startLine && got.EndLine == endLine && got.Kind == kind {
			return
		}
	}
	t.Fatalf("expected folding range %d-%d kind=%q, got %#v", startLine, endLine, kind, ranges)
}
