package hover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkHoverMethodChain(b *testing.B) {
	read := func(rel string) string {
		content, err := os.ReadFile(filepath.Join(testdataPath(), rel))
		if err != nil {
			b.Fatal(err)
		}
		return string(content)
	}
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for _, rel := range []string{"vendor/monolog/monolog/src/Monolog/Logger.php", "vendor/monolog/monolog/src/Monolog/Handler/StreamHandler.php", "src/Service.php"} {
		idx.IndexFile("file:///project/"+rel, read(rel))
	}
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, testdataPath(), "none"), "none")
	source := read("src/Service.php")
	pos := protocol.Position{Line: 19, Character: 33}
	b.ReportAllocs()
	for b.Loop() {
		if p.GetHover("file:///project/src/Service.php", source, pos) == nil {
			b.Fatal("info() has a hover")
		}
	}
}
