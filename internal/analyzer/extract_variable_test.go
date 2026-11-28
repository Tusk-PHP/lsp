package analyzer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
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
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor.extract"}},
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
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
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
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor.extract"}},
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
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor.extract"}},
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

func TestGetCodeActionsExtractVariableAfterMultibyteText(t *testing.T) {
	source := "<?php\nfunction run($user) {\n    return 'é' . strtoupper($user);\n}\n"
	edits := extractVariableEdits(t, source, protocol.Range{
		Start: protocol.Position{Line: 2, Character: 18},
		End:   protocol.Position{Line: 2, Character: 35},
	})
	if len(edits) != 2 || edits[0].NewText != "    $extracted = strtoupper($user);\n" {
		t.Fatalf("edits = %+v, want strtoupper($user) extracted", edits)
	}
}

func TestGetCodeActionsExtractVariableRefusesStatementsAndAssignments(t *testing.T) {
	for name, tc := range map[string]struct {
		line       string
		start, end int
	}{
		"assignment": {"    $a = $b;", 4, 11},
		"compound":   {"    $a ??= $b;", 4, 13},
		"statement":  {"    $a; $b;", 4, 11},
		"unbalanced": {"    f(g($a), 1);", 6, 12},
		"comment":    {"    f(/* x */);", 6, 13},
	} {
		t.Run(name, func(t *testing.T) {
			source := "<?php\nfunction run($a, $b) {\n" + tc.line + "\n}\n"
			rng := protocol.Range{Start: protocol.Position{Line: 2, Character: tc.start}, End: protocol.Position{Line: 2, Character: tc.end}}
			if edits := extractVariableEdits(t, source, rng); edits != nil {
				t.Fatalf("did not expect an extract action, got %+v", edits)
			}
		})
	}
}

func TestGetCodeActionsExtractVariableInsertsBeforeTheStatement(t *testing.T) {
	source := "<?php\nfunction run($a) {\n    foo(1);\n    $x = [bar(2), ($a)];\n}\n"
	edits := extractVariableEdits(t, source, protocol.Range{
		Start: protocol.Position{Line: 3, Character: 10},
		End:   protocol.Position{Line: 3, Character: 16},
	})
	if len(edits) != 2 || edits[0].Range.Start.Line != 3 || edits[0].NewText != "    $extracted = bar(2);\n" {
		t.Fatalf("edits = %+v, want bar(2) extracted on line 3", edits)
	}
}

func extractVariableEdits(t *testing.T, source string, rng protocol.Range) []protocol.TextEdit {
	t.Helper()
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range:        rng,
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor.extract"}},
	})
	for _, action := range actions {
		if action.Kind == "refactor.extract" {
			return action.Edit.Changes["file:///test.php"]
		}
	}
	return nil
}

func TestGetCodeActionsExtractVariableStepsOverNestedGroups(t *testing.T) {
	source := "<?php\nfunction run($b) {\n    f(g(1), [2], $b + 1);\n}\n"
	edits := extractVariableEdits(t, source, protocol.Range{
		Start: protocol.Position{Line: 2, Character: 17},
		End:   protocol.Position{Line: 2, Character: 23},
	})
	if len(edits) != 2 || edits[0].Range.Start.Line != 2 || edits[0].NewText != "    $extracted = $b + 1;\n" {
		t.Fatalf("edits = %+v, want $b + 1 extracted before line 2", edits)
	}
}
