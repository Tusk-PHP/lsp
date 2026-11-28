package lsp

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestBladeCompletionEditsMoveToTheView(t *testing.T) {
	doc := blade.Compile("@can('ed')\n")
	line, col, ok := doc.PHPPosition(0, 6)
	if !ok {
		t.Fatal("the ability string is in the generated PHP")
	}
	items := bladeRequest{uri: "file:///v.blade.php", doc: doc}.completions([]protocol.CompletionItem{
		{Label: "edit", TextEdit: &protocol.TextEdit{Range: protocol.Range{Start: protocol.Position{Line: line, Character: col}, End: protocol.Position{Line: line, Character: col + 2}}, NewText: "edit"}},
		{Label: "generated", TextEdit: &protocol.TextEdit{Range: protocol.Range{End: protocol.Position{Character: 1}}, NewText: "generated"}},
	})
	if e := items[0].TextEdit; e == nil || e.Range.Start != (protocol.Position{Line: 0, Character: 6}) || e.Range.End != (protocol.Position{Line: 0, Character: 8}) {
		t.Errorf("an edit in the view moves to it: %+v", items[0].TextEdit)
	}
	if items[1].TextEdit != nil || items[1].InsertText != "generated" {
		t.Errorf("an edit in generated PHP becomes the inserted text: %+v", items[1])
	}
}
