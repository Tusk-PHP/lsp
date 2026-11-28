package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnABuiltinConstantShowsItsValue(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	src := "<?php\n$max = PHP_INT_MAX;\n"
	h := p.GetHover("file:///a.php", src, protocol.Position{Line: 1, Character: 10})
	if h == nil || !strings.Contains(h.Contents.Value, "PHP_INT_MAX") || !strings.Contains(h.Contents.Value, "9223372036854775807") {
		t.Errorf("hover on PHP_INT_MAX shows its value: %+v", h)
	}
}
