package analyzer

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/rule"
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
	want := map[string]protocol.TextEdit{
		"Ignore undefined-variable on this line":    {Range: pointRange(2, 0), NewText: "    // @tusk-ignore undefined-variable\n"},
		"Ignore argument.type with @phpstan-ignore": {Range: pointRange(3, len("\tstrlen(1);")), NewText: " // @phpstan-ignore argument.type"},
		"Ignore pint:braces on this line":           {Range: pointRange(3, 0), NewText: "\t// @tusk-ignore pint:braces\n"},
	}
	if len(actions) != len(want) {
		t.Fatalf("got %d actions, want %d: %+v", len(actions), len(want), actions)
	}
	for _, a := range actions {
		if edit := singleEdit(t, a, uri); a.Kind != protocol.CodeActionQuickFix || edit != want[a.Title] {
			t.Errorf("%q: kind %q, edit %+v", a.Title, a.Kind, edit)
		}
	}
}

func pointRange(line, character int) protocol.Range {
	at := protocol.Position{Line: line, Character: character}
	return protocol.Range{Start: at, End: at}
}

func TestIgnoreDiagnosticActionsForPHPStanEndTheLine(t *testing.T) {
	uri := "file:///app/Shop.php"
	source := "<?php\nclass Shop\n{\n    /**\n     * @return int\n     */\n    #[NoSuchAttr]\n    public function total(): int { return $this->sum(); }\n    public function tax(): int { return $this->rate('é'); } // keep this note\n    public function name(): string { return \"a\n    b\"; }\n}\n?>\n<p><?php $this->x() ?></p>\n"
	cases := []struct {
		line int
		want []protocol.TextEdit
	}{
		{6, []protocol.TextEdit{{Range: pointRange(6, len("    #[NoSuchAttr]")), NewText: " // @phpstan-ignore method.notFound"}}},
		{7, []protocol.TextEdit{{Range: pointRange(7, len("    public function total(): int { return $this->sum(); }")), NewText: " // @phpstan-ignore method.notFound"}}},
		{8, []protocol.TextEdit{{Range: pointRange(8, len("    public function tax(): int { return $this->rate('é'); } ")), NewText: "/* @phpstan-ignore method.notFound */ "}}},
		{9, nil},
		{12, nil},
	}
	for _, c := range cases {
		actions := IgnoreDiagnosticActions(uri, source, ignoreParams(uri, nil, diagAt(c.line, "phpstan", "method.notFound")))
		var got []protocol.TextEdit
		for _, a := range actions {
			got = append(got, singleEdit(t, a, uri))
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("line %d: got %+v, want %+v", c.line, got, c.want)
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
	actions := BladeIgnoreDiagnosticActions(uri, source, ignoreParams(uri, nil, diagAt(1, "tusk-php", "unknown-function")), nil)
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

func TestIgnoreDiagnosticActionsGoAboveDocblocksAndAttributes(t *testing.T) {
	uri := "file:///app/Shop.php"
	source := "<?php\nclass Shop\n{\n    /**\n     * @return Cart\n     */\n    #[Route('/cart')]\n    public function cart() {}\n    public function pay() {} /** @return int */\n}\n"
	actions := IgnoreDiagnosticActions(uri, source, ignoreParams(uri, nil,
		diagAt(7, "tusk-php", "unknown-class"),
		diagAt(7, "tusk-php", "unknown-member"),
		diagAt(8, "tusk-php", "unknown-member"),
	))
	lines := map[string][]int{}
	for _, a := range actions {
		edit := singleEdit(t, a, uri)
		lines[a.Title] = append(lines[a.Title], edit.Range.Start.Line)
	}
	if got := lines["Ignore unknown-class on this line"]; len(got) != 1 || got[0] != 3 {
		t.Errorf("the marker goes above the docblock and attribute: %v", lines)
	}
	if got := lines["Ignore unknown-member on this line"]; len(got) != 2 || got[0] != 3 || got[1] != 8 {
		t.Errorf("each line gets its own marker; a docblock after code doesn't move it: %v", lines)
	}
}

func TestIgnoreDiagnosticActionsOfferEachPintFixer(t *testing.T) {
	uri := "file:///app/A.php"
	source := "<?php\n$a  =  'x';\n"
	actions := IgnoreDiagnosticActions(uri, source, ignoreParams(uri, nil, diagAt(1, "pint", rule.PintCode([]string{"binary_operator_spaces", "single_quote"}))))
	var titles []string
	for _, a := range actions {
		titles = append(titles, a.Title)
	}
	if !slices.Equal(titles, []string{"Ignore pint:binary_operator_spaces on this line", "Ignore pint:single_quote on this line"}) {
		t.Errorf("got %v", titles)
	}
}

func TestIgnoreDiagnosticActionsInBladePHPBlocksUsePHPComments(t *testing.T) {
	uri := "file:///resources/views/a.blade.php"
	source := "@php\r\n    nope();\r\n@endphp\r\n"
	actions := BladeIgnoreDiagnosticActions(uri, source, ignoreParams(uri, nil, diagAt(1, "tusk-php", "unknown-function")), nil)
	if len(actions) != 1 || singleEdit(t, actions[0], uri).NewText != "    // @tusk-ignore unknown-function\r\n" {
		t.Errorf("a PHP comment in the file's own line ending: %+v", actions)
	}
}

func TestIgnoreDiagnosticActionsWithoutDiagnostics(t *testing.T) {
	uri := "file:///app/A.php"
	if got := IgnoreDiagnosticActions(uri, "<?php\n$a;\n", ignoreParams(uri, nil)); got == nil || len(got) != 0 {
		t.Errorf("got %#v", got)
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
