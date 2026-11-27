package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
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
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
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
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
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
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
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

func TestGetCodeActionsGenerateConstructorFindsTheClassBody(t *testing.T) {
	for name, tc := range map[string]struct {
		source string
		line   int
		want   protocol.Position
		indent string
	}{
		"after a class constant": {"<?php\necho Foo::class; class A { private $x; }\n", 1, protocol.Position{Line: 1, Character: 39}, "    "},
		"brace in a string":      {"<?php\nclass A {\n    private $x;\n    public function f() { return \"}{$y}\"; }\n}\n", 1, protocol.Position{Line: 4, Character: 0}, "    "},
		"indent from a method":   {"<?php\nclass A {\nprivate $x;\n\tpublic function f() {}\n}\n", 1, protocol.Position{Line: 4, Character: 0}, "\t"},
	} {
		t.Run(name, func(t *testing.T) {
			edit, ok := generatedConstructorEdit(t, tc.source, tc.line)
			if !ok {
				t.Fatal("expected Generate Constructor action")
			}
			if edit.Range.Start != tc.want || !strings.Contains(edit.NewText, tc.indent+"public function __construct(") {
				t.Fatalf("edit = %+v, want insertion at %v indented by %q", edit, tc.want, tc.indent)
			}
		})
	}
	for name, tc := range map[string]struct {
		source string
		line   int
	}{
		"unterminated body":     {"<?php\nclass A { private $x;\n", 1},
		"name on the next line": {"<?php\nclass\nA { private $x; }\n", 2},
	} {
		t.Run(name, func(t *testing.T) {
			if edit, ok := generatedConstructorEdit(t, tc.source, tc.line); ok {
				t.Fatalf("did not expect a Generate Constructor action, got %+v", edit)
			}
		})
	}
}

func generatedConstructorEdit(t *testing.T, source string, line int) (protocol.TextEdit, bool) {
	t.Helper()
	a := setupCoverageAnalyzer()
	actions := a.GetCodeActions("file:///app/A.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/A.php"},
		Range:        protocol.Range{Start: protocol.Position{Line: line}, End: protocol.Position{Line: line}},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
	})
	for _, action := range actions {
		if action.Title == "Generate Constructor" {
			return action.Edit.Changes["file:///app/A.php"][0], true
		}
	}
	return protocol.TextEdit{}, false
}
