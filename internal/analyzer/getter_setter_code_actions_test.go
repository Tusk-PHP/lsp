package analyzer

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

func TestGetCodeActionsGenerateGetterSetter(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
namespace App;

class User {
    private string $name;
}
`

	actions := a.GetCodeActions("file:///app/User.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/User.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 4, Character: 4},
			End:   protocol.Position{Line: 4, Character: 24},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor"}},
	})

	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(actions))
	}

	var combined, getterOnly, setterOnly *protocol.CodeAction
	for i := range actions {
		action := &actions[i]
		switch action.Title {
		case "Generate getter and setter for $name":
			combined = action
		case "Generate getter for $name":
			getterOnly = action
		case "Generate setter for $name":
			setterOnly = action
		}
		if action.Kind != "refactor" {
			t.Fatalf("expected refactor kind, got %q", action.Kind)
		}
	}

	if combined == nil || combined.Edit == nil {
		t.Fatal("expected combined getter/setter action with edit")
	}
	combinedText := combined.Edit.Changes["file:///app/User.php"][0].NewText
	if !strings.Contains(combinedText, "public function getName(): string") {
		t.Fatalf("combined edit missing getter:\n%s", combinedText)
	}
	if !strings.Contains(combinedText, "public function setName(string $name): void") {
		t.Fatalf("combined edit missing setter:\n%s", combinedText)
	}

	if getterOnly == nil || getterOnly.Edit == nil {
		t.Fatal("expected getter-only action")
	}
	getterText := getterOnly.Edit.Changes["file:///app/User.php"][0].NewText
	if strings.Contains(getterText, "setName(") {
		t.Fatalf("getter-only edit should not contain setter:\n%s", getterText)
	}

	if setterOnly == nil || setterOnly.Edit == nil {
		t.Fatal("expected setter-only action")
	}
	setterText := setterOnly.Edit.Changes["file:///app/User.php"][0].NewText
	if strings.Contains(setterText, "getName(") {
		t.Fatalf("setter-only edit should not contain getter:\n%s", setterText)
	}
}

func TestGetCodeActionsGenerateGetterSetterSkipsExistingMethods(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
namespace App;

class User {
    private string $name;

    public function getName(): string {
        return $this->name;
    }
}
`

	actions := a.GetCodeActions("file:///app/User.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/User.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 4, Character: 4},
			End:   protocol.Position{Line: 4, Character: 24},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor"}},
	})

	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Title != "Generate setter for $name" {
		t.Fatalf("unexpected action title %q", actions[0].Title)
	}
}

func TestGetCodeActionsGenerateGetterSetterSkipsReadonlySetter(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
namespace App;

class User {
    public readonly string $name;
}
`

	actions := a.GetCodeActions("file:///app/User.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/User.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 4, Character: 4},
			End:   protocol.Position{Line: 4, Character: 33},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor"}},
	})

	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Title != "Generate getter for $name" {
		t.Fatalf("unexpected action title %q", actions[0].Title)
	}
}

func TestGetCodeActionsGenerateGetterSetterSkipsUnsafeProperties(t *testing.T) {
	a := setupCoverageAnalyzer()
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "static property",
			source: `<?php
namespace App;

class User {
    public static string $name;
}
`,
		},
		{
			name: "property hooks",
			source: `<?php
namespace App;

class User {
    public string $name {
        get => $this->name;
    }
}
`,
		},
		{
			name: "multiple properties on line",
			source: `<?php
namespace App;

class User {
    private string $first, $last;
}
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actions := a.GetCodeActions("file:///app/User.php", tc.source, protocol.CodeActionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/User.php"},
				Range: protocol.Range{
					Start: protocol.Position{Line: 4, Character: 4},
					End:   protocol.Position{Line: 4, Character: 30},
				},
				Context: protocol.CodeActionContext{Only: []string{"refactor"}},
			})
			if len(actions) != 0 {
				t.Fatalf("expected no actions, got %#v", actions)
			}
		})
	}
}
