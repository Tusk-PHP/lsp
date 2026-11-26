package checks

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func BenchmarkUnusedPrivateRule(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nclass Service {\n")
	for i := range 40 {
		n := strconv.Itoa(i)
		src.WriteString("    private int $count" + n + " = 0;\n    private const LIMIT" + n + " = " + n + ";\n")
	}
	for i := range 40 {
		n := strconv.Itoa(i)
		src.WriteString("    private function step" + n + "(array $items): int {\n")
		src.WriteString("        // Walks the items and counts them.\n")
		for range 10 {
			src.WriteString("        $this->count" + n + " += count($items) + self::LIMIT" + n + ";\n")
			src.WriteString("        $label = \"step {$this->count" + n + "} of \" . strlen('items');\n")
		}
		src.WriteString("        return $this->count" + n + ";\n    }\n")
		src.WriteString("    public function run" + n + "(array $items): int { return $this->step" + n + "($items); }\n")
	}
	src.WriteString("}\n")
	source := src.String()
	file := parser.ParseFile(source)
	r := &UnusedPrivateRule{Shared: &Shared{}}
	r.Shared.parse(source)
	b.ReportAllocs()
	for b.Loop() {
		if len(r.Check(file, source, nil)) != 0 {
			b.Fatal("every private member of the benchmark class is used")
		}
	}
}
