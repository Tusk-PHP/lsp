package analyzer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestGetCodeActionsCopyReferenceOnSymbol(t *testing.T) {
	source := `<?php
namespace App;

class Widget {
    public function render(): string {
        return '';
    }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///app/Widget.php": source})

	// Cursor on the class name "Widget" (line 3, character 6).
	actions := a.GetCodeActions("file:///app/Widget.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/Widget.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 6},
			End:   protocol.Position{Line: 3, Character: 12},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor"}},
	})

	var found *protocol.CodeAction
	for i := range actions {
		if actions[i].Title == "Copy reference..." {
			found = &actions[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected 'Copy reference...' action, got %#v", actions)
	}
	if found.Kind != "refactor" {
		t.Fatalf("expected kind 'refactor', got %q", found.Kind)
	}
	if found.Command == nil {
		t.Fatal("expected Command to be set")
	}
	if found.Command.Command != "tuskPhpLsp.copyReference" {
		t.Fatalf("expected command 'tuskPhpLsp.copyReference', got %q", found.Command.Command)
	}
	if len(found.Command.Arguments) != 2 {
		t.Fatalf("expected 2 command arguments, got %d", len(found.Command.Arguments))
	}
}

func TestGetCodeActionsCopyReferenceNotOnSymbol(t *testing.T) {
	source := `<?php
namespace App;

class Widget {
    public function render(): string {
        return '';
    }
}

`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///app/Widget.php": source})

	// Cursor on a blank line (line 8) — not a symbol.
	actions := a.GetCodeActions("file:///app/Widget.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/Widget.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 8, Character: 0},
			End:   protocol.Position{Line: 8, Character: 0},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor"}},
	})

	for _, action := range actions {
		if action.Title == "Copy reference..." {
			t.Fatalf("unexpected 'Copy reference...' action on blank line: %#v", action)
		}
	}
}

func TestGetCodeActionsCopyReferenceKindGating(t *testing.T) {
	source := `<?php
namespace App;

class Widget {
    public function render(): string {
        return '';
    }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///app/Widget.php": source})

	// Request only "quickfix" — refactor actions must not appear.
	actions := a.GetCodeActions("file:///app/Widget.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/Widget.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 6},
			End:   protocol.Position{Line: 3, Character: 12},
		},
		Context: protocol.CodeActionContext{Only: []string{"quickfix"}},
	})

	for _, action := range actions {
		if action.Title == "Copy reference..." {
			t.Fatalf("'Copy reference...' must not appear when Only=[quickfix]: %#v", action)
		}
	}
}
