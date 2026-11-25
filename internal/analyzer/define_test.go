package analyzer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestDefinitionOfConstantDeclaredWithDefine(t *testing.T) {
	idx := symbols.NewIndex()
	const bootURI = "file:///project/bootstrap.php"
	idx.IndexFile(bootURI, "<?php\nif (!defined('APP_VERSION')) {\n    define('APP_VERSION', '1.2');\n}\n")
	a := NewAnalyzer(idx, nil)
	src := "<?php\nnamespace App;\necho APP_VERSION;\n"
	loc := a.FindDefinition("file:///project/src/x.php", src, protocol.Position{Line: 2, Character: 7})
	if loc == nil || loc.URI != bootURI || loc.Range.Start != (protocol.Position{Line: 2, Character: 12}) {
		t.Fatalf("go to definition opens the define() call's name: %+v", loc)
	}
}
