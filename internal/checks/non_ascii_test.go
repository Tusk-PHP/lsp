package checks

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestAnImportIsNotUsedByALongerNonASCIIName(t *testing.T) {
	source := "<?php\nuse App\\Models\\Caf;\nuse App\\Models\\Größe;\n\n$a = new Café();\n$b = new Größe();\n"
	findings := (&UnusedImportsRule{}).Check(parser.ParseFile(source, token.Latest), source, nil)
	assertFindingCodes(t, findings, []string{"unused-import"})
	if len(findings) == 1 && findings[0].Message != "Unused import 'App\\Models\\Caf'" {
		t.Errorf("Café doesn't use the import Caf, Größe uses Größe: %s", findings[0].Message)
	}
}
