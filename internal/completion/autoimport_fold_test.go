package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestAutoImportFoldsASCIICaseOnly(t *testing.T) {
	p := NewProvider(symbols.NewIndex(), nil, "")
	file := &parser.FileNode{Namespace: "App", Uses: []parser.UseNode{{FullName: `Other\Ätna`, Alias: "Ätna"}}}
	if name, edits := p.newImportPlanner("", file).class(`Other\ÄTNA`); name != "Ätna" || edits != nil {
		t.Errorf("an imported class in another ASCII case keeps its import: got %q %v", name, edits)
	}
	name, edits := p.newImportPlanner("", file).class(`Lib\ätna`)
	if name != "ätna" || len(edits) != 1 {
		t.Errorf("ätna is not the imported Ätna, so it is imported as itself: got %q %v", name, edits)
	}
	if name, edits := p.newImportPlanner("", file).class(`App\ätna`); name != "ätna" || edits != nil {
		t.Errorf("ätna in the file's own namespace needs no import: got %q %v", name, edits)
	}
}
