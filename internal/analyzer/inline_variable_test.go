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

func inlinedSource(t *testing.T, source string, pos protocol.Position) string {
	t.Helper()
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range:        protocol.Range{Start: pos, End: pos},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor.inline"}},
	})
	for _, action := range actions {
		if action.Kind != "refactor.inline" || action.Edit == nil {
			continue
		}
		result := source
		for _, edit := range action.Edit.Changes["file:///test.php"] {
			start, okStart := positionToOffset(result, edit.Range.Start)
			end, okEnd := positionToOffset(result, edit.Range.End)
			if !okStart || !okEnd {
				t.Fatalf("edit %+v is outside the source", edit)
			}
			result = result[:start] + edit.NewText + result[end:]
		}
		return result
	}
	t.Fatal("expected a refactor.inline code action")
	return ""
}

func dynamicScopeSource(statement string) string {
	return "<?php\nfunction run($data, $query, $name, $args) {\n    $label = 'x';\n    " + statement + "\n    return $label;\n}\n"
}

var dynamicScopeLabelUse = protocol.Position{Line: 4, Character: 12}

func TestGetCodeActionsInlineVariableRejectsDynamicScopeAccess(t *testing.T) {
	for name, statement := range map[string]string{
		"extract":                       "extract($data);",
		"fully qualified extract":       "\\extract($data);",
		"extract in upper case":         "EXTRACT($data);",
		"parse_str without a result":    "parse_str($query);",
		"parse_str with a spread":       "parse_str(...$args);",
		"parse_str unclosed":            "parse_str($query",
		"variable variable write":       "$$name = 1;",
		"variable variable read":        "echo $$name;",
		"braced variable variable":      "echo ${'label'};",
		"compact":                       "$all = compact('label');",
		"get_defined_vars":              "$all = get_defined_vars();",
		"eval":                          "eval('echo $label;');",
		"compact in an arrow function":  "$all = (fn() => compact('label'))();",
		"variable variable as property": "echo $data->$$name;",
	} {
		t.Run(name, func(t *testing.T) {
			assertNoInlineVariableAction(t, dynamicScopeSource(statement), dynamicScopeLabelUse)
		})
	}
}

func TestGetCodeActionsInlineVariableIgnoresDynamicScopeAccessElsewhere(t *testing.T) {
	for name, statement := range map[string]string{
		"names in a string":           "echo 'extract($data) compact(\"label\") $$name ${label}';",
		"names in a comment":          "// extract($data); eval($query); $$name",
		"names in a block comment":    "/* get_defined_vars() ${'label'} */",
		"parse_str with a result":     "parse_str($query, $parsed);",
		"parse_str with a named one":  "parse_str(string: $query, result: $parsed);",
		"parse_str with nested calls": "parse_str(trim(substr($query, 1)), $parsed[0]);",
		"parse_str with a comment":    "parse_str(/* raw */ $query, $parsed);",
		"a method named extract":      "$data->extract($query);",
		"a static method named eval":  "Loader::eval($query);",
		"a function named compact":    "$all = new compact($query);",
		"a static variable property":  "echo Loader::$$name;",
		"a constant named extract":    "echo extract;",
		"extract in a closure":        "$fn = function () use ($data) { extract($data); };",
		"get_defined_vars in a named": "function inner() { return get_defined_vars(); }",
	} {
		t.Run(name, func(t *testing.T) {
			got := inlinedSource(t, dynamicScopeSource(statement), dynamicScopeLabelUse)
			want := "<?php\nfunction run($data, $query, $name, $args) {\n    " + statement + "\n    return 'x';\n}\n"
			if got != want {
				t.Errorf("inlined source = %q, want %q", got, want)
			}
		})
	}
}

func TestGetCodeActionsInlineVariableRejectsDynamicScopeAccessAtFileScope(t *testing.T) {
	for name, statement := range map[string]string{
		"extract":           "extract($data);",
		"variable variable": "$$name = 1;",
		"compact":           "$all = compact('label');",
	} {
		t.Run(name, func(t *testing.T) {
			assertNoInlineVariableAction(t, "<?php\n$label = 'x';\n"+statement+"\necho $label;\n", protocol.Position{Line: 3, Character: 7})
		})
	}
}
