package resolve

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func BenchmarkFunctionCallTypeInANamespaceBlock(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\n")
	for n := range 20 {
		ns := "Block" + strconv.Itoa(n)
		src.WriteString("namespace " + ns + ";\nuse function Lib\\helper;\nclass Made {}\nfunction make(): Made {}\n")
		for range 50 {
			src.WriteString("$x = make();\nhelper();\n")
		}
	}
	src.WriteString("$last = make();\n")
	source := src.String()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///blocks.php", source)
	file := parser.ParseFile(source)
	lines := SplitLines(source)
	r := NewResolver(idx)
	pos := textpos.Position{Line: len(lines) - 1}
	b.ReportAllocs()
	for b.Loop() {
		if got := r.ResolveVariableTypeTyped("$last", lines, pos, file).FullString(); got != `Block19\Made` {
			b.Fatalf("$last = %q", got)
		}
	}
}
