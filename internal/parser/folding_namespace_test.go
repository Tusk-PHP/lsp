package parser

import (
	"slices"
	"testing"
)

func TestFoldingNamespaceOperatorIsNotADeclaration(t *testing.T) {
	source := "<?php\nnamespace App;\n\nnamespace\\foo();\nfunction bar()\n{\n    return 1;\n}\n"
	want := []FoldingRange{{StartLine: 1, EndLine: 7}, {StartLine: 4, EndLine: 6}}
	if got := ExtractFoldingRanges(source); !slices.Equal(got, want) {
		t.Errorf("ExtractFoldingRanges = %+v, want %+v", got, want)
	}
}

func TestFoldingAdjacentClasses(t *testing.T) {
	source := "<?php\nclass A\n{\n    public $a;\n}\nclass B\n{\n    public $b;\n}\n"
	want := []FoldingRange{{StartLine: 1, EndLine: 3}, {StartLine: 5, EndLine: 7}}
	if got := ExtractFoldingRanges(source); !slices.Equal(got, want) {
		t.Errorf("ExtractFoldingRanges = %+v, want %+v", got, want)
	}
}
