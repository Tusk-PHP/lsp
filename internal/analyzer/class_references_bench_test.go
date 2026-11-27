package analyzer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkClassReferences(b *testing.B) {
	sources := map[string]string{"file:///app/Models/User.php": "<?php\nnamespace App\\Models;\nclass User {}\n"}
	for i := range 400 {
		var src strings.Builder
		fmt.Fprintf(&src, "<?php\nnamespace App\\Services%d;\n\n", i)
		if i%2 == 0 {
			src.WriteString("use App\\Models\\User;\n")
		}
		fmt.Fprintf(&src, "\nclass Service%d\n{\n", i)
		for m := range 40 {
			if i%2 == 0 {
				fmt.Fprintf(&src, "    public function run%d(User $user): User\n    {\n        return new User();\n    }\n\n", m)
			} else {
				fmt.Fprintf(&src, "    public function run%d(array $rows): int\n    {\n        return count($rows);\n    }\n\n", m)
			}
		}
		src.WriteString("}\n")
		sources[fmt.Sprintf("file:///app/Services%d/Service.php", i)] = src.String()
	}
	idx := symbols.NewIndex()
	for uri, src := range sources {
		idx.IndexFileWithSource(uri, src, symbols.SourceProject)
	}
	a := NewAnalyzer(idx, nil)
	const uri = "file:///app/Models/User.php"
	pos := protocol.Position{Line: 2, Character: 7}
	read := func(u string) string { return sources[u] }
	if got := len(a.FindAllReferences(uri, sources[uri], pos, read)); got != 1+200*(1+40*3) {
		b.Fatalf("references = %d", got)
	}
	b.ReportAllocs()
	for b.Loop() {
		a.FindAllReferences(uri, sources[uri], pos, read)
	}
}
