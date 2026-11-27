package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

const adjacentClassesSource = `<?php
namespace App;

class A implements Countable {
    private string $a;
}
class B implements Countable {
    private string $b;
}
class C implements Countable {} class D implements Countable {}
`

func refactorActionsAt(t *testing.T, a *Analyzer, line, character int) []protocol.CodeAction {
	t.Helper()
	return a.GetCodeActions("file:///classes.php", adjacentClassesSource, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///classes.php"},
		Range:        protocol.Range{Start: protocol.Position{Line: line, Character: character}, End: protocol.Position{Line: line, Character: character}},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
	})
}

func adjacentClassesAnalyzer() *Analyzer {
	a, _ := setupRenameAnalyzer(map[string]string{
		"file:///classes.php":   adjacentClassesSource,
		"file:///countable.php": "<?php\nnamespace App;\n\ninterface Countable {\n    public function count(): int;\n}\n",
	})
	return a
}

func editOf(t *testing.T, actions []protocol.CodeAction, title string) protocol.TextEdit {
	t.Helper()
	for _, action := range actions {
		if action.Title == title && action.Edit != nil {
			if edits := action.Edit.Changes["file:///classes.php"]; len(edits) == 1 {
				return edits[0]
			}
		}
	}
	t.Fatalf("no %q action with one edit in %#v", title, actions)
	return protocol.TextEdit{}
}

func TestGetterSetterIsInsertedBeforeTheClassClosingBrace(t *testing.T) {
	a := adjacentClassesAnalyzer()
	for _, c := range []struct {
		line, closingBrace int
		title              string
	}{
		{4, 5, "Generate getter for $a"},
		{7, 8, "Generate getter for $b"},
	} {
		edit := editOf(t, refactorActionsAt(t, a, c.line, 4), c.title)
		if edit.Range.Start.Line != c.closingBrace {
			t.Errorf("%s inserted at line %d, want the closing brace's line %d, inside the class", c.title, edit.Range.Start.Line, c.closingBrace)
		}
	}
}

func TestImplementMissingMethodsPicksTheClassAtTheCursor(t *testing.T) {
	a := adjacentClassesAnalyzer()
	for _, c := range []struct {
		name            string
		line, character int
		insertLine      int
	}{
		{"the first class", 3, 7, 5},
		{"the first line of the next class", 6, 7, 8},
	} {
		edit := editOf(t, refactorActionsAt(t, a, c.line, c.character), "Implement missing methods")
		if edit.Range.Start.Line != c.insertLine || !strings.Contains(edit.NewText, "public function count(): int") {
			t.Errorf("%s: edit at line %d with %q, want count() at line %d", c.name, edit.Range.Start.Line, edit.NewText, c.insertLine)
		}
	}
}
