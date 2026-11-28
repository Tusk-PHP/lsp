package parser

import (
	"slices"
	"testing"
)

func TestFoldingDeclarationsStartAtTheirKeywordAndClosuresAtTheirBrace(t *testing.T) {
	source := `<?php
/*
 * block
 */
interface Shape
{
    public function area();
}
trait Named
{
    public $name;
}
enum Suit
{
    case Hearts;
}
function &byRef(
    array $a
) {
    return $a;
}
$f = function (
    $a
) {
    return $a;
};
`
	want := []FoldingRange{
		{StartLine: 1, EndLine: 3, Kind: FoldingRangeKindComment},
		{StartLine: 4, EndLine: 6},
		{StartLine: 8, EndLine: 10},
		{StartLine: 12, EndLine: 14},
		{StartLine: 16, EndLine: 19},
		{StartLine: 23, EndLine: 24, Kind: FoldingRangeKindRegion},
	}
	if got := ExtractFoldingRanges(source); !slices.Equal(got, want) {
		t.Errorf("ExtractFoldingRanges = %+v, want %+v", got, want)
	}
}

func TestFoldingSurvivesUnbalancedDelimiters(t *testing.T) {
	source := "<?php\nfunction f()\n{\n    $a = [1, 2;\n    return $a;\n}\n}\n"
	want := []FoldingRange{{StartLine: 1, EndLine: 4}}
	if got := ExtractFoldingRanges(source); !slices.Equal(got, want) {
		t.Errorf("ExtractFoldingRanges = %+v, want %+v", got, want)
	}
}

func TestFoldingFileEndingAfterADeclarationKeyword(t *testing.T) {
	for _, source := range []string{"<?php\nfunction", "<?php\nnamespace App\n"} {
		if got := ExtractFoldingRanges(source); len(got) != 0 {
			t.Errorf("ExtractFoldingRanges(%q) = %+v, want none", source, got)
		}
	}
}
