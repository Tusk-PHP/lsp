package completion

import (
	"fmt"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// BenchmarkClassNameCompletion completes a class name that matches 500
// classes across namespaces, each needing an import decision.
func BenchmarkClassNameCompletion(b *testing.B) {
	idx := symbols.NewIndex()
	for i := 0; i < 500; i++ {
		src := fmt.Sprintf("<?php\nnamespace Vendor\\Pkg%d\\Models;\nclass User%d {}\n", i, i)
		idx.IndexFileWithSource(fmt.Sprintf("file:///v/%d.php", i), src, symbols.SourceVendor)
	}
	p := NewProvider(idx, nil, "")
	src := "<?php\nnamespace App\\Http\\Controllers;\n\nuse App\\Models\\User;\nuse App\\Models\\Post;\n\nclass HomeController\n{\n    public function index()\n    {\n        new User"
	pos := protocol.Position{Line: 10, Character: 16}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.GetCompletions("file:///app/Http/Controllers/HomeController.php", src, pos)
	}
}
