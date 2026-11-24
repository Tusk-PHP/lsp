package parser

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func FuzzParse(f *testing.F) {
	for _, src := range programs {
		f.Add(src, uint8(token.Latest))
	}
	f.Fuzz(func(t *testing.T, src string, minor uint8) {
		checkTree(t, src, token.Version(minor%uint8(token.Latest+1)))
	})
}
