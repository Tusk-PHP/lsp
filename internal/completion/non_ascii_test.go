package completion

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestCompletionOffersNonASCIINames(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Atna.php", "<?php\nnamespace App;\nclass Ätna { public function größe(): int { return 1; } }\nfunction café() {}\n")
	p := NewProvider(idx, nil, "")
	for src, want := range map[string]string{
		"<?php\nnamespace App;\n$a = new Ätna();\n$a->gr": "größe",
		"<?php\nnamespace App;\ncaf":                      "café",
		"<?php\nnamespace App;\nnew Ät":                   "Ätna",
	} {
		lines := strings.Split(src, "\n")
		last := lines[len(lines)-1]
		pos := protocol.Position{Line: len(lines) - 1, Character: len(last)}
		found := false
		for _, item := range p.GetCompletions("file:///use.php", src, pos) {
			found = found || item.Label == want
		}
		if !found {
			t.Errorf("completion after %q offers %s", last, want)
		}
	}
}
