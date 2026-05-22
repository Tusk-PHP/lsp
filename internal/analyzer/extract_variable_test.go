package analyzer

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

func TestGetCodeActionsExtractVariable(t *testing.T) {
	source := `<?php
function run($user) {
    return strtoupper($user);
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 2, Character: 11},
			End:   protocol.Position{Line: 2, Character: 28},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor.extract"}},
	})

	var action *protocol.CodeAction
	for i := range actions {
		if actions[i].Kind == "refactor.extract" {
			action = &actions[i]
			break
		}
	}
	if action == nil {
		t.Fatal("expected refactor.extract code action")
	}
	if action.Edit == nil {
		t.Fatal("expected extract variable workspace edit")
	}

	edits := action.Edit.Changes["file:///test.php"]
	if len(edits) != 2 {
		t.Fatalf("expected 2 edits, got %d", len(edits))
	}
	if edits[0].NewText != "    $extracted = strtoupper($user);\n" {
		t.Fatalf("unexpected insertion edit: %q", edits[0].NewText)
	}
	if edits[1].NewText != "$extracted" {
		t.Fatalf("unexpected replacement edit: %q", edits[1].NewText)
	}
}

func TestGetCodeActionsExtractVariableAllowedByRefactorParent(t *testing.T) {
	source := `<?php
function run($user) {
    return strtoupper($user);
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 2, Character: 11},
			End:   protocol.Position{Line: 2, Character: 28},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor"}},
	})

	for _, action := range actions {
		if action.Kind == "refactor.extract" {
			return
		}
	}

	t.Fatalf("expected refactor.extract action when only=refactor, got %#v", actions)
}

func TestGetCodeActionsExtractVariableSkipsArrowFunctionBody(t *testing.T) {
	source := `<?php
function run($user) {
    $mapper = fn () => strtoupper($user);
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 2, Character: 23},
			End:   protocol.Position{Line: 2, Character: 40},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor.extract"}},
	})

	for _, action := range actions {
		if action.Kind == "refactor.extract" {
			t.Fatal("did not expect refactor.extract inside arrow-function body")
		}
	}
}

func TestGetCodeActionsExtractVariableResolvesNameConflict(t *testing.T) {
	source := `<?php
function run($user) {
    $extracted = "taken";
    return strtoupper($user);
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 11},
			End:   protocol.Position{Line: 3, Character: 28},
		},
		Context: protocol.CodeActionContext{Only: []string{"refactor.extract"}},
	})

	for _, action := range actions {
		if action.Kind != "refactor.extract" {
			continue
		}
		edits := action.Edit.Changes["file:///test.php"]
		if edits[0].NewText != "    $extracted1 = strtoupper($user);\n" {
			t.Fatalf("unexpected insertion edit: %q", edits[0].NewText)
		}
		if edits[1].NewText != "$extracted1" {
			t.Fatalf("unexpected replacement edit: %q", edits[1].NewText)
		}
		return
	}

	t.Fatal("expected refactor.extract code action")
}
