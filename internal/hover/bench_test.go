package hover

import (
	"os"
	"path/filepath"
	"strings"
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

func BenchmarkHoverMemberChain(b *testing.B) {
	root := testdataPath()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for _, rel := range []string{"vendor/monolog/monolog/src/Monolog/Logger.php", "vendor/monolog/monolog/src/Monolog/Handler/StreamHandler.php"} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			b.Fatal(err)
		}
		idx.IndexFile("file:///project/"+rel, string(data))
	}
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, root, "none"), "none")
	src := "<?php\nuse Monolog\\Logger;\nuse Monolog\\Handler\\StreamHandler;\nclass Report {\n    public function run(Logger $logger): void {\n        $handler = new StreamHandler('php://stderr');\n        $named = Logger::create('report');\n        $handler->getLogger()->info($named->error('x') ? 'done' : ')');\n    }\n}\n"
	line := 7
	pos := protocol.Position{Line: line, Character: strings.Index(strings.Split(src, "\n")[line], "info")}
	if h := p.GetHover("file:///project/src/Report.php", src, pos); h == nil || !strings.Contains(h.Contents.Value, "function info") {
		b.Fatalf("hover on info: %+v", h)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		p.GetHover("file:///project/src/Report.php", src, pos)
	}
}
