package analyzer

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkDefinitionContainerArg(b *testing.B) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///Request.php", "<?php\nnamespace Illuminate\\Http;\nclass Request {}\n")
	ca := container.NewContainerAnalyzer(idx, b.TempDir(), "laravel")
	ca.Analyze()
	a := NewAnalyzer(idx, ca)
	var body strings.Builder
	body.WriteString("<?php\nfunction run(array $rows) {\n")
	for i := 0; i < 200; i++ {
		body.WriteString("    $v" + strconv.Itoa(i) + " = $rows[" + strconv.Itoa(i) + "] ?? null;\n")
	}
	body.WriteString("    return app('request');\n}\n")
	src := body.String()
	pos := protocol.Position{Line: 202, Character: strings.Index("    return app('request');", "quest")}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if a.FindDefinition("file:///s.php", src, pos) == nil {
			b.Fatal("no definition")
		}
	}
}
