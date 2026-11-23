package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func ignoreParams(uri string, only []protocol.CodeActionKind, diags ...protocol.Diagnostic) protocol.CodeActionParams {
	return protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Context:      protocol.CodeActionContext{Diagnostics: diags, Only: only},
	}
}

func diagAt(line int, source, code string) protocol.Diagnostic {
	return protocol.Diagnostic{Range: protocol.Range{Start: protocol.Position{Line: line, Character: 4}}, Source: source, Code: code}
}

func singleEdit(t *testing.T, action protocol.CodeAction, uri string) protocol.TextEdit {
	t.Helper()
	edits := action.Edit.Changes[uri]
	if len(edits) != 1 {
		t.Fatalf("%s: want one edit, got %+v", action.Title, edits)
	}
	return edits[0]
}

func TestIgnoreDiagnosticActionsInsertCommentAbove(t *testing.T) {
	uri := "file:///app/A.php"
	source := "<?php\nfunction f() {\n    echo $x;\n\tstrlen(1);\n}\n"
	actions := IgnoreDiagnosticActions(uri, source, ignoreParams(uri, nil,
		diagAt(2, "tusk-php", "undefined-variable"),
		diagAt(2, "tusk-php", "undefined-variable"),
		diagAt(3, "phpstan", "argument.type"),
		diagAt(3, "pint", "braces"),
	))
	want := map[string]string{
		"Ignore undefined-variable on this line":    "    // @tusk-ignore undefined-variable\n",
		"Ignore argument.type with @phpstan-ignore": "\t// @phpstan-ignore argument.type\n",
		"Ignore pint:braces on this line":           "\t// @tusk-ignore pint:braces\n",
	}
	if len(actions) != len(want) {
		t.Fatalf("got %d actions, want %d: %+v", len(actions), len(want), actions)
	}
	for _, a := range actions {
		edit := singleEdit(t, a, uri)
		if a.Kind != "quickfix" || edit.NewText != want[a.Title] || edit.Range.Start.Character != 0 {
			t.Errorf("%q: kind %q, edit %+v", a.Title, a.Kind, edit)
		}
	}
}

func TestIgnoreDiagnosticActionsOnlyWherePHPCodeStarts(t *testing.T) {
	uri := "file:///app/view.php"
	source := "<?php $a->b();\n?>\n<p><?= $x ?></p>\n<?php\n$s = <<<EOT\n{$y}\nEOT;\n$z();\n"
	actions := IgnoreDiagnosticActions(uri, source, ignoreParams(uri, nil,
		diagAt(0, "tusk-php", "undefined-variable"),
		diagAt(2, "tusk-php", "undefined-variable"),
		diagAt(5, "tusk-php", "undefined-variable"),
		diagAt(7, "tusk-php", "undefined-variable"),
	))
	if len(actions) != 1 || singleEdit(t, actions[0], uri).Range.Start.Line != 7 {
		t.Errorf("only line 7 starts in PHP code outside a string, got %+v", actions)
	}
}

func TestIgnoreDiagnosticActionsInBladeUseBladeComments(t *testing.T) {
	uri := "file:///resources/views/a.blade.php"
	source := "<div>\n  {{ missing() }}\n</div>\n"
	actions := IgnoreDiagnosticActions(uri, source, ignoreParams(uri, nil, diagAt(1, "tusk-php", "unknown-function")))
	if len(actions) != 1 || singleEdit(t, actions[0], uri).NewText != "  {{-- @tusk-ignore unknown-function --}}\n" {
		t.Errorf("got %+v", actions)
	}
}

func TestIgnoreDiagnosticActionsNeedAKnownCode(t *testing.T) {
	uri := "file:///app/A.php"
	source := "<?php\n$a;\n"
	got := IgnoreDiagnosticActions(uri, source, ignoreParams(uri, nil,
		diagAt(1, "tusk-php", ""),
		diagAt(1, "psalm", "MixedAssignment"),
		diagAt(1, "tusk-php", "not-a-rule"),
		diagAt(9, "tusk-php", "unused-variable"),
	))
	if got == nil || len(got) != 0 {
		t.Errorf("no action for a missing code, an unknown tool or rule, or a line past the end; got %#v", got)
	}
}

func TestIgnoreDiagnosticActionsRespectOnly(t *testing.T) {
	uri := "file:///app/A.php"
	source := "<?php\n$a;\n"
	if got := IgnoreDiagnosticActions(uri, source, ignoreParams(uri, []protocol.CodeActionKind{"refactor"}, diagAt(1, "tusk-php", "unused-variable"))); len(got) != 0 {
		t.Errorf("quick fixes are not refactorings, got %+v", got)
	}
}

func withoutIgnoreActions(actions []protocol.CodeAction) []protocol.CodeAction {
	kept := actions[:0:0]
	for _, a := range actions {
		if !strings.HasPrefix(a.Title, "Ignore ") {
			kept = append(kept, a)
		}
	}
	return kept
}
