package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestSelfReferentialAssignmentsStopThroughTheChainHook(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	for _, c := range []struct{ source, expr string }{
		{"<?php\n$x = $x->foo();\n$x->", "$x"},
		{"<?php\n$a = $b->x();\n$b = $a->y();\n$a->", "$a"},
	} {
		source, expr := c.source, c.expr
		file := parser.ParseFile(source, token.Latest)
		pos := protocol.Position{Line: 3}
		if got := p.ResolveExpressionType(expr+"->foo()", source, pos, file); got != "" {
			t.Errorf("%q: got %q, want nothing", source, got)
		}
		if got := p.ResolveExpressionTypeTyped(expr, source, pos, file); !got.IsEmpty() {
			t.Errorf("%q typed: got %+v, want nothing", source, got)
		}
	}
}
