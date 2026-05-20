package parser

import "testing"

func TestExtractFoldingRanges(t *testing.T) {
	source := `<?php
namespace App;

/**
 * Summary
 * More details
 */
class Example
{
    public function build(): array
    {
        $items = [
            'a' => 1,
            'b' => 2,
        ];

        if (true) {
            return $items;
        }
    }
}

// first
// second
function helper(): void
{
    $values = [
        1,
        2,
    ];
}
`

	ranges := ExtractFoldingRanges(source)
	assertFoldRange(t, ranges, 1, 30, "")
	assertFoldRange(t, ranges, 3, 6, FoldingRangeKindComment)
	assertFoldRange(t, ranges, 7, 19, "")
	assertFoldRange(t, ranges, 9, 18, "")
	assertFoldRange(t, ranges, 11, 13, FoldingRangeKindRegion)
	assertFoldRange(t, ranges, 16, 17, FoldingRangeKindRegion)
	assertFoldRange(t, ranges, 22, 23, FoldingRangeKindComment)
	assertFoldRange(t, ranges, 24, 29, "")
	assertFoldRange(t, ranges, 26, 28, FoldingRangeKindRegion)
}

func TestExtractFoldingRangesBracedNamespaceAndAttributes(t *testing.T) {
	source := `<?php
namespace App\Feature {
    #[Route(
        '/users',
    )]
    final class Controller {
        public function __invoke() {
            return [
                'ok' => true,
            ];
        }
    }
}
`

	ranges := ExtractFoldingRanges(source)
	assertFoldRange(t, ranges, 1, 11, "")
	assertFoldRange(t, ranges, 5, 10, "")
	assertFoldRange(t, ranges, 6, 9, "")
	assertFoldRange(t, ranges, 7, 8, FoldingRangeKindRegion)
	assertNoFoldRange(t, ranges, 2, 4)
}

func assertFoldRange(t *testing.T, ranges []FoldingRange, startLine, endLine int, kind FoldingRangeKind) {
	t.Helper()
	for _, got := range ranges {
		if got.StartLine == startLine && got.EndLine == endLine && got.Kind == kind {
			return
		}
	}
	t.Fatalf("expected folding range %d-%d kind=%q, got %#v", startLine, endLine, kind, ranges)
}

func assertNoFoldRange(t *testing.T, ranges []FoldingRange, startLine, endLine int) {
	t.Helper()
	for _, got := range ranges {
		if got.StartLine == startLine && got.EndLine == endLine {
			t.Fatalf("unexpected folding range %d-%d: %#v", startLine, endLine, ranges)
		}
	}
}
