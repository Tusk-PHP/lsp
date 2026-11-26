package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnNonASCIINames(t *testing.T) {
	decl := "<?php\nnamespace App;\nclass Ätna { public function größe(): int { return 1; } }\nfunction café() {}\n"
	use := "<?php\nnamespace App;\n$a = new Ätna();\n$a->größe();\ncafé();\n"
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///Atna.php", decl)
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	for line, want := range map[int]string{2: `App\Ätna`, 3: "function größe", 4: "function café"} {
		col := strings.Index(strings.Split(use, "\n")[line], "Ä")
		if line == 3 {
			col = len("$a->")
		}
		if line == 4 {
			col = 0
		}
		h := p.GetHover("file:///use.php", use, protocol.Position{Line: line, Character: col})
		if h == nil || !strings.Contains(h.Contents.Value, want) {
			t.Errorf("hover on line %d: got %+v, want %q", line, h, want)
		}
	}
}
