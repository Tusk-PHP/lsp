package laravel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkRouteIndexScanWorkspace(b *testing.B) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "laravel"))
	if err != nil {
		b.Fatal(err)
	}
	idx := symbols.NewIndex()
	paths, err := filepath.Glob(filepath.Join(root, "routes", "*.php"))
	if err != nil || len(paths) == 0 {
		b.Fatalf("route files of %s: %v", root, err)
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		idx.IndexFile("file://"+path, string(content))
	}
	routes := NewRouteIndex(root, idx.GrammarOf)
	b.ReportAllocs()
	for b.Loop() {
		if err := routes.ScanWorkspace(); err != nil {
			b.Fatal(err)
		}
	}
	if len(routes.Names()) == 0 {
		b.Fatal("no routes found")
	}
}
