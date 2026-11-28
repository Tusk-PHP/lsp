package graph

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func BenchmarkBuildReferencesLaravel(b *testing.B) {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "laravel"))
	if err != nil {
		b.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "vendor", "laravel", "framework")); err != nil {
		b.Skip("testdata/laravel/vendor is missing: run `composer install --working-dir testdata/laravel`")
	}
	ws := workspace.New(workspace.Options{RootPath: root, Framework: "laravel"})
	ws.IndexWorkspace()
	ws.IndexComposerDependencies()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		BuildReferences(ws.Index, Options{})
	}
}
