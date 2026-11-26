package parser

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/phpref"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func BenchmarkParse(b *testing.B) {
	sources := phpref.ReadAll(b, phpref.Corpus(b))
	size := 0
	for _, src := range sources {
		size += len(src)
	}
	b.Run("php/parser", func(b *testing.B) {
		b.SetBytes(int64(size))
		b.ReportAllocs()
		for b.Loop() {
			for _, src := range sources {
				Parse(src, token.Latest)
			}
		}
	})
}
