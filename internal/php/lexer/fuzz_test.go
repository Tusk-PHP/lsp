package lexer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func FuzzLex(f *testing.F) {
	for _, s := range append(append(snippets, malformedSnippets...), versionSnippets...) {
		f.Add(s, uint8(token.Latest))
	}
	f.Fuzz(func(t *testing.T, src string, minor uint8) {
		var b strings.Builder
		for _, tok := range Lex(src, token.Version(minor%uint8(token.Latest+1))) {
			if tok.End <= tok.Start {
				t.Fatalf("empty token %s at %d", tok.Kind, tok.Start)
			}
			b.WriteString(tok.Text(src))
		}
		if b.String() != src {
			t.Fatalf("not lossless: %q", src)
		}
	})
}
