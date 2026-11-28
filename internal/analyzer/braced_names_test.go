package analyzer

import (
	"sort"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func bracedNameSource(statement string) string {
	return "<?php\nfunction run($name) {\n    $label = 'x';\n    " + statement + "\n    return $label;\n}\n"
}

func bracedNameLabelUse(source string) protocol.Position {
	return protocol.Position{Line: strings.Count(source[:strings.Index(source, "return $label")], "\n"), Character: 12}
}

func TestRenameVariableRenamesBracedNamesInStrings(t *testing.T) {
	for name, tc := range map[string]struct{ statement, want string }{
		"double quotes": {`echo "${label}";`, `echo "${title}";`},
		"offset":        {`echo "${label['k']}${label[0]}";`, `echo "${title['k']}${title[0]}";`},
		"heredoc":       {"echo <<<EOT\n${label}\nEOT;", "echo <<<EOT\n${title}\nEOT;"},
		"mixed":         {`echo "$label ${label}";`, `echo "$title ${title}";`},
	} {
		t.Run(name, func(t *testing.T) {
			source := bracedNameSource(tc.statement)
			a, reader := setupRenameAnalyzer(map[string]string{"file:///test.php": source})
			edit := a.Rename("file:///test.php", source, bracedNameLabelUse(source), "$title", reader)
			if edit == nil {
				t.Fatal("expected a rename of $label")
			}
			edits := append([]protocol.TextEdit(nil), edit.Changes["file:///test.php"]...)
			sort.SliceStable(edits, func(i, j int) bool {
				return comparePosition(edits[i].Range.Start, edits[j].Range.Start) > 0
			})
			got := source
			for _, e := range edits {
				start, okStart := positionToOffset(got, e.Range.Start)
				end, okEnd := positionToOffset(got, e.Range.End)
				if !okStart || !okEnd {
					t.Fatalf("edit %+v is outside the source", e)
				}
				got = got[:start] + e.NewText + got[end:]
			}
			want := "<?php\nfunction run($name) {\n    $title = 'x';\n    " + tc.want + "\n    return $title;\n}\n"
			if got != want {
				t.Errorf("renamed source = %q, want %q", got, want)
			}
		})
	}
}

func TestGetCodeActionsInlineVariableRejectsBracedNamesInStrings(t *testing.T) {
	for name, statement := range map[string]string{
		"braced name":              `echo "${label}";`,
		"braced name with offset":  `echo "${label['k']}";`,
		"computed name":            `echo "${$name}";`,
		"computed name in heredoc": "echo <<<EOT\n${strtolower($name)}\nEOT;",
		"computed in an arrow fn":  `$f = fn() => "${$name}";`,
	} {
		t.Run(name, func(t *testing.T) {
			source := bracedNameSource(statement)
			assertNoInlineVariableAction(t, source, bracedNameLabelUse(source))
		})
	}
}

func TestGetCodeActionsInlineVariableIgnoresComputedNamesInAClosure(t *testing.T) {
	source := bracedNameSource(`$f = function () use ($name) { return "${$name}"; };`)
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range:        protocol.Range{Start: bracedNameLabelUse(source), End: bracedNameLabelUse(source)},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor.inline"}},
	})
	for _, action := range actions {
		if action.Kind == "refactor.inline" {
			return
		}
	}
	t.Fatal("expected a refactor.inline code action: the closure's ${…} reads only the closure's variables")
}
