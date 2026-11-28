package symbols

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func TestIndexAcceptsDeclarationsWithAnEmptyDocBlock(t *testing.T) {
	for _, decl := range []string{"interface Source {}", "class Source {}", "trait Source {}"} {
		file := parser.ParseFile("<?php\n/**\n */\n" + decl + "\n")
		syms := fileDeclarations("file:///source.php", file)
		if len(syms) != 1 || syms[0].Name != "Source" || len(syms[0].Children) != 0 {
			t.Errorf("%s with an empty doc block: symbols = %+v, want Source without members", decl, syms)
		}
	}
}
