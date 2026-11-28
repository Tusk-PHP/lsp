package checks

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkArgumentCountInNamespaceBlocks(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\n")
	for n := range 20 {
		src.WriteString("namespace Block" + strconv.Itoa(n) + ";\nuse function Lib\\helper;\nfunction make($a, $b = 1) {}\n")
		for range 50 {
			src.WriteString("make(1);\nhelper(2);\nstrlen('x');\n")
		}
	}
	source := src.String()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///lib.php", "<?php\nnamespace Lib;\nfunction helper($n) {}\n")
	idx.IndexFile("file:///blocks.php", source)
	file := parser.ParseFile(source)
	b.ReportAllocs()
	for b.Loop() {
		if got := (&ArgumentCountRule{}).Check(file, source, idx); len(got) != 0 {
			b.Fatalf("unexpected findings: %+v", got)
		}
	}
}
