package lexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func benchCorpus(b *testing.B) []string {
	b.Helper()
	var sources []string
	_ = filepath.WalkDir("../../../testdata/laravel/vendor", func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".php") {
			if data, err := os.ReadFile(path); err == nil {
				sources = append(sources, string(data))
			}
		}
		return nil
	})
	if len(sources) == 0 {
		b.Skip("the Laravel fixture's vendor tree isn't installed")
	}
	return sources
}

func BenchmarkLex(b *testing.B) {
	sources := benchCorpus(b)
	size := 0
	for _, src := range sources {
		size += len(src)
	}
	b.Run("lexer", func(b *testing.B) {
		b.SetBytes(int64(size))
		b.ReportAllocs()
		for b.Loop() {
			for _, src := range sources {
				Lex(src, token.Latest)
			}
		}
	})
	b.Run("parser.Tokenize", func(b *testing.B) {
		b.SetBytes(int64(size))
		b.ReportAllocs()
		for b.Loop() {
			for _, src := range sources {
				parser.Tokenize(src)
			}
		}
	})
}
