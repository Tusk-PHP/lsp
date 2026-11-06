package diagnostics

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestAnalyzeTreatsPHTMLAsIncludedTemplate(t *testing.T) {
	src := "<?php\necho $block->getTitle();\n"
	p := NewProvider(symbols.NewIndex(), "none", "/tmp", nil, config.DefaultConfig())
	count := func(uri string) int {
		n := 0
		for _, d := range p.Analyze(uri, src) {
			if strings.Contains(d.Message, "Undefined variable $block") {
				n++
			}
		}
		return n
	}
	if n := count("file:///view/templates/title.phtml"); n != 0 {
		t.Errorf(".phtml template: $block reported %d times, want 0", n)
	}
	if n := count("file:///src/title.php"); n != 1 {
		t.Errorf(".php file: $block reported %d times, want 1", n)
	}
}
