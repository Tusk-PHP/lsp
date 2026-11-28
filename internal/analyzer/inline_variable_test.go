package analyzer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
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
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor.inline"}},
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
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor.inline"}},
	})

	for _, action := range actions {
		if action.Kind == "refactor.inline" {
			t.Fatal("did not expect refactor.inline code action")
		}
	}
}

func TestGetCodeActionsInlineVariableRejectsFileScope(t *testing.T) {
	t.Run("on the open tag's line", func(t *testing.T) {
		assertNoInlineVariableAction(t, "<?php $a = 1; echo $a;\n", protocol.Position{Line: 0, Character: 20})
	})
	t.Run("after the open tag on its own line", func(t *testing.T) {
		assertNoInlineVariableAction(t, "<?php\n$label = 'x' . 'y';\necho $label;\n", protocol.Position{Line: 2, Character: 7})
	})
}

func TestGetCodeActionsInlineVariableRejectsArrowFunctionBody(t *testing.T) {
	source := "<?php\nfunction run() {\n    return fn() => ($sum = 1) + $sum;\n}\n"
	assertNoInlineVariableAction(t, source, protocol.Position{Line: 2, Character: 34})
}

func TestGetCodeActionsInlineVariableRejectsIncrementAndCoalesceAssignment(t *testing.T) {
	for name, use := range map[string]string{
		"post-increment": "$label++;",
		"pre-decrement":  "--$label;",
		"coalesce":       "$label ??= 'x';",
	} {
		t.Run(name, func(t *testing.T) {
			source := "<?php\nfunction run($name) {\n    $label = $name;\n    " + use + "\n    return $label;\n}\n"
			assertNoInlineVariableAction(t, source, protocol.Position{Line: 4, Character: 12})
		})
	}
}

func TestGetCodeActionsInlineVariableRejectsUnsafeExpressions(t *testing.T) {
	for name, expr := range map[string]string{
		"increment":    "$name++",
		"shell":        "`ls`",
		"variable var": "$$name",
		"isset":        "isset($name)",
		"method":       "$name->trim()",
		"keyword":      "print $name",
	} {
		t.Run(name, func(t *testing.T) {
			source := "<?php\nfunction run($name) {\n    $label = " + expr + ";\n    return $label;\n}\n"
			assertNoInlineVariableAction(t, source, protocol.Position{Line: 3, Character: 12})
		})
	}
}

func TestGetCodeActionsInlineVariableKeepsAtomicExpressionsBare(t *testing.T) {
	for expr, want := range map[string]string{
		`"hi {$name}"`:  `"hi {$name}"`,
		"0x1F":          "0x1F",
		"($name . '!')": "($name . '!')",
		"$name == 1":    "($name == 1)",
		"null":          "(null)",
	} {
		t.Run(expr, func(t *testing.T) {
			source := "<?php\nfunction run($name) {\n    $label = " + expr + ";\n    return $label;\n}\n"
			edits := inlineVariableEdits(t, source, protocol.Position{Line: 3, Character: 12})
			if len(edits) != 2 || edits[0].NewText != want {
				t.Fatalf("edits = %+v, want the use replaced by %q", edits, want)
			}
		})
	}
}

func TestGetCodeActionsInlineVariableRejectsUseInsideAString(t *testing.T) {
	source := "<?php\nfunction run($name) {\n    $label = $name;\n    return \"$label!\";\n}\n"
	assertNoInlineVariableAction(t, source, protocol.Position{Line: 2, Character: 6})
}

func TestGetCodeActionsInlineVariableDeletesAMultiLineAssignment(t *testing.T) {
	source := "<?php\nfunction run($name) {\n    $label = $name\n        . '!';\n    return $label;\n}\n"
	edits := inlineVariableEdits(t, source, protocol.Position{Line: 4, Character: 12})
	want := protocol.Range{Start: protocol.Position{Line: 2}, End: protocol.Position{Line: 3, Character: 14}}
	if len(edits) != 2 || edits[1].NewText != "" || edits[1].Range != want {
		t.Fatalf("edits = %+v, want the assignment deleted over %v", edits, want)
	}
}

func TestGetCodeActionsInlineVariableAfterMultibyteText(t *testing.T) {
	source := "<?php\nfunction run($name) {\n    $label = 'é' . $name;\n    return $label;\n}\n"
	edits := inlineVariableEdits(t, source, protocol.Position{Line: 3, Character: 12})
	if len(edits) != 2 || edits[0].NewText != "('é' . $name)" {
		t.Fatalf("edits = %+v, want the use replaced by ('é' . $name)", edits)
	}
}

func inlineVariableEdits(t *testing.T, source string, pos protocol.Position) []protocol.TextEdit {
	t.Helper()
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range:        protocol.Range{Start: pos, End: pos},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor.inline"}},
	})
	for _, action := range actions {
		if action.Kind == "refactor.inline" {
			return action.Edit.Changes["file:///test.php"]
		}
	}
	t.Fatal("expected refactor.inline code action")
	return nil
}
