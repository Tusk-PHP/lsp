package analyzer

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

func TestGetCodeActionsInlineVariable(t *testing.T) {
	source := `<?php
function run($name) {
    $label = $name . "!";
    return $label;
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range:        protocol.Range{Start: protocol.Position{Line: 3, Character: 12}, End: protocol.Position{Line: 3, Character: 12}},
		Context:      protocol.CodeActionContext{Only: []string{"refactor.inline"}},
	})

	var action *protocol.CodeAction
	for i := range actions {
		if actions[i].Kind == "refactor.inline" {
			action = &actions[i]
			break
		}
	}
	if action == nil {
		t.Fatal("expected refactor.inline code action")
	}
	if action.Edit == nil {
		t.Fatal("expected inline variable workspace edit")
	}

	edits := action.Edit.Changes["file:///test.php"]
	if len(edits) != 2 {
		t.Fatalf("expected 2 edits, got %d", len(edits))
	}

	foundDelete := false
	foundReplace := false
	for _, edit := range edits {
		switch {
		case edit.NewText == "":
			foundDelete = true
		case edit.NewText == "($name . \"!\")":
			foundReplace = true
		}
	}
	if !foundDelete {
		t.Fatal("expected deletion of the original assignment statement")
	}
	if !foundReplace {
		t.Fatal("expected parenthesized inline replacement")
	}
}

func TestGetCodeActionsInlineVariableRejectsReassignment(t *testing.T) {
	source := `<?php
function run($name) {
    $label = $name;
    $label = trim($label);
    return $label;
}
`
	assertNoInlineVariableAction(t, source, protocol.Position{Line: 4, Character: 12})
}

func TestGetCodeActionsInlineVariableRejectsCompoundMutation(t *testing.T) {
	source := `<?php
function run($name) {
    $label = $name;
    $label .= "!";
    return $label;
}
`
	assertNoInlineVariableAction(t, source, protocol.Position{Line: 4, Character: 12})
}

func TestGetCodeActionsInlineVariableRejectsConditionalAssignment(t *testing.T) {
	source := `<?php
function run($name, $enabled) {
    if ($enabled) {
        $label = $name;
    }
    return $label;
}
`
	assertNoInlineVariableAction(t, source, protocol.Position{Line: 5, Character: 12})
}

func TestGetCodeActionsInlineVariableRejectsSideEffects(t *testing.T) {
	source := `<?php
function run($name) {
    $label = trim($name);
    return $label;
}
`
	assertNoInlineVariableAction(t, source, protocol.Position{Line: 3, Character: 12})
}

func TestGetCodeActionsInlineVariableRejectsClosureCapture(t *testing.T) {
	source := `<?php
function run($name) {
    $label = $name;
    $fn = function () use ($label) {
        return $label;
    };
    return $fn();
}
`
	assertNoInlineVariableAction(t, source, protocol.Position{Line: 3, Character: 30})
}

func assertNoInlineVariableAction(t *testing.T, source string, pos protocol.Position) {
	t.Helper()

	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range:        protocol.Range{Start: pos, End: pos},
		Context:      protocol.CodeActionContext{Only: []string{"refactor.inline"}},
	})

	for _, action := range actions {
		if action.Kind == "refactor.inline" {
			t.Fatal("did not expect refactor.inline code action")
		}
	}
}
