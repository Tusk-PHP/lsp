package analyzer

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

func TestGetCodeActionsGenerateConstructor(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
namespace App;

class User {
    private string $name;
    private ?int $age;
    private static string $slug;
    private string $label = 'ready';
}
`

	actions := a.GetCodeActions("file:///app/User.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/User.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 0},
			End:   protocol.Position{Line: 3, Character: 10},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor"}},
	})

	var constructor *protocol.CodeAction
	for i := range actions {
		if actions[i].Title == "Generate Constructor" {
			constructor = &actions[i]
			break
		}
	}
	if constructor == nil {
		t.Fatalf("expected Generate Constructor action, got %#v", actions)
	}
	if constructor.Kind != "refactor" {
		t.Fatalf("expected refactor kind, got %q", constructor.Kind)
	}
	if constructor.Edit == nil {
		t.Fatal("expected constructor action edit")
	}

	edits := constructor.Edit.Changes["file:///app/User.php"]
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %#v", edits)
	}
	if edits[0].Range.Start.Line != 8 || edits[0].Range.Start.Character != 0 {
		t.Fatalf("unexpected insert range %#v", edits[0].Range)
	}

	newText := edits[0].NewText
	if !strings.Contains(newText, "public function __construct(string $name, ?int $age)") {
		t.Fatalf("constructor signature missing expected params:\n%s", newText)
	}
	if !strings.Contains(newText, "$this->name = $name;") || !strings.Contains(newText, "$this->age = $age;") {
		t.Fatalf("constructor body missing assignments:\n%s", newText)
	}
	if strings.Contains(newText, "$slug") || strings.Contains(newText, "$label") {
		t.Fatalf("constructor should skip static/defaulted properties:\n%s", newText)
	}
}

func TestGetCodeActionsGenerateConstructorSkipsExistingConstructor(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
namespace App;

class User {
    private string $name;

    public function __construct(string $name) {
        $this->name = $name;
    }
}
`

	actions := a.GetCodeActions("file:///app/User.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/User.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 0},
			End:   protocol.Position{Line: 3, Character: 10},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor"}},
	})

	for _, action := range actions {
		if action.Title == "Generate Constructor" {
			t.Fatalf("unexpected constructor action: %#v", action)
		}
	}
}

func TestGetCodeActionsGenerateConstructorOneLineClass(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
class User { private string $name; }
`

	actions := a.GetCodeActions("file:///app/User.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/User.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 1, Character: 0},
			End:   protocol.Position{Line: 1, Character: 10},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor"}},
	})

	var constructor *protocol.CodeAction
	for i := range actions {
		if actions[i].Title == "Generate Constructor" {
			constructor = &actions[i]
			break
		}
	}
	if constructor == nil || constructor.Edit == nil {
		t.Fatalf("expected constructor action for one-line class, got %#v", actions)
	}

	edit := constructor.Edit.Changes["file:///app/User.php"][0]
	if edit.Range.Start.Line != 1 {
		t.Fatalf("expected same-line insertion, got %#v", edit.Range)
	}
	if !strings.HasPrefix(edit.NewText, "\n    public function __construct(string $name)") {
		t.Fatalf("unexpected one-line constructor text:\n%s", edit.NewText)
	}
}
