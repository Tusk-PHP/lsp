package analyzer

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func actionTitles(actions []protocol.CodeAction) []string {
	titles := make([]string, 0, len(actions))
	for _, a := range actions {
		titles = append(titles, string(a.Kind)+" "+a.Title)
	}
	return titles
}

func TestGetCodeActionsBuildsOnlyTheRequestedKinds(t *testing.T) {
	source := `<?php
namespace App\Models;

use App\Unused\Thing;

class Order
{
    private int $total;
    private string $name;

    public function sum(): int
    {
        return $this->total + 1;
    }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///app/Models/Order.php": source})
	a.SetClientCommands([]protocol.CommandName{protocol.CommandCopyReference})
	params := func(only []protocol.CodeActionKind, line int) protocol.CodeActionParams {
		pos := protocol.Position{Line: line, Character: 8}
		return protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/Models/Order.php"},
			Range:        protocol.Range{Start: pos, End: pos},
			Context: protocol.CodeActionContext{Only: only, Diagnostics: []protocol.Diagnostic{{
				Range:  protocol.Range{Start: protocol.Position{Line: 3, Character: 4}, End: protocol.Position{Line: 3, Character: 20}},
				Source: "tusk-php", Code: "unused-import", Message: "unused",
			}}},
		}
	}
	for _, line := range []int{3, 5, 7} {
		all := a.GetCodeActions("file:///app/Models/Order.php", source, params(nil, line))
		kinds := map[protocol.CodeActionKind]bool{}
		for _, action := range all {
			kinds[action.Kind] = true
		}
		if len(kinds) < 3 {
			t.Fatalf("line %d: the test needs several kinds of action, got %v", line, actionTitles(all))
		}
		for _, only := range [][]protocol.CodeActionKind{
			{protocol.CodeActionQuickFix},
			{protocol.CodeActionRefactor},
			{protocol.CodeActionRefactorMove},
			{protocol.CodeActionRefactorExtract},
			{protocol.CodeActionSource},
			{protocol.CodeActionSourceOrganizeImports},
			{protocol.CodeActionQuickFix, protocol.CodeActionSource},
		} {
			var want []protocol.CodeAction
			for _, action := range all {
				if action.Kind.In(only) {
					want = append(want, action)
				}
			}
			got := a.GetCodeActions("file:///app/Models/Order.php", source, params(only, line))
			if !slices.Equal(actionTitles(got), actionTitles(want)) {
				t.Errorf("line %d, only %v:\n got %v\nwant %v", line, only, actionTitles(got), actionTitles(want))
			}
		}
	}
}
