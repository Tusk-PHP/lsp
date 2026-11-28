package checks

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkUnknownClassInNamespaceBlocks(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\n")
	for n := range 20 {
		src.WriteString("namespace Block" + strconv.Itoa(n) + ";\nuse Lib\\Thing;\nclass Local implements \\Countable { public function count(): int { return 0; } }\n")
		for range 50 {
			src.WriteString("$a = new Thing();\n$b = new Local();\nThing::make();\ntry {} catch (\\RuntimeException $e) {}\n")
		}
	}
	source := src.String()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///lib.php", "<?php\nnamespace Lib;\nclass Thing { public static function make() {} }\n")
	idx.IndexFile("file:///blocks.php", source)
	file := parser.ParseFile(source)
	b.ReportAllocs()
	for b.Loop() {
		if got := (&UnknownClassRule{}).Check(file, source, idx); len(got) != 0 {
			b.Fatalf("unexpected findings: %+v", got)
		}
	}
}
