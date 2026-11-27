package laravel

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkValidationScans(b *testing.B) {
	root := filepath.Join("..", "..", "..", "testdata", "laravel", "vendor")
	var sources []string
	size := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".php") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil {
			sources = append(sources, string(data))
			size += len(data)
		}
		return nil
	})
	if len(sources) == 0 {
		b.Skip("testdata/laravel/vendor is missing: run `composer install --working-dir testdata/laravel`")
	}
	b.Run("extensions", func(b *testing.B) {
		b.SetBytes(int64(size))
		b.ReportAllocs()
		for b.Loop() {
			e := NewValidationExtensions()
			for _, src := range sources {
				e.IndexFile("file:///f.php", src)
			}
		}
	})
	b.Run("helpers", func(b *testing.B) {
		b.SetBytes(int64(size))
		b.ReportAllocs()
		for b.Loop() {
			h := NewValidationHelpers()
			for _, src := range sources {
				h.IndexFile("file:///f.php", src)
			}
		}
	})
	b.Run("index", func(b *testing.B) {
		b.SetBytes(int64(size))
		b.ReportAllocs()
		for b.Loop() {
			idx := symbols.NewIndex()
			for _, src := range sources {
				idx.IndexFileWithSource("file:///f.php", src, symbols.SourceVendor)
			}
		}
	})
}
