package parser

import (
	"strings"
	"testing"
)

func BenchmarkExtractFoldingRanges(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App\\Services;\n\nclass Handler\n{\n")
	for i := range 300 {
		src.WriteString("    /**\n     * Runs.\n     */\n    public function run")
		src.WriteString(strings.Repeat("x", i%7+1))
		src.WriteString("(): array\n    {\n        return [\n            'a' => 1,\n        ];\n    }\n\n")
	}
	src.WriteString("}\n")
	source := src.String()
	b.ReportAllocs()
	for b.Loop() {
		ExtractFoldingRanges(source)
	}
}
