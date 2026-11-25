package completion

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkArrayKeysFromAMethodCall(b *testing.B) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///repo.php", shapeLibrary["file:///repo.php"])
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nclass Controller {\n")
	for i := range 300 {
		fmt.Fprintf(&src, "    public function m%d(Repo $repo): int { return count($repo->load()); }\n", i)
	}
	src.WriteString("    public function show(Repo $repo) {\n        $r = $repo->load();\n        $r['")
	source := src.String()
	pos := protocol.Position{Line: strings.Count(source, "\n"), Character: len("        $r['")}
	p := NewProvider(idx, nil, "")
	b.ReportAllocs()
	for b.Loop() {
		p.GetCompletions("file:///app/use.php", source, pos)
	}
}
