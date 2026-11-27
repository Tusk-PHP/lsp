package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestCompletionImportsAgainstTheNamespaceBlockAroundTheCursor(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///x/Dep.php", "<?php\nnamespace X;\nclass Dep {}\n", symbols.SourceProject)
	idx.IndexFileWithSource("file:///b/Dep.php", "<?php\nnamespace B;\nclass Dep {}\n", symbols.SourceProject)
	p := NewProvider(idx, nil, "")
	for _, tc := range []struct {
		name, source string
	}{
		{"unbraced", "<?php\nnamespace A;\nuse X\\Dep;\nclass Foo { function f() { new Dep(); } }\n\nnamespace B;\nclass Bar { function g() { new Dep"},
		{"braced", "<?php\nnamespace A {\nuse X\\Dep;\nclass Foo { function f() { new Dep(); } }\n}\nnamespace B {\nclass Bar { function g() { new Dep"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := importText(completeAt(t, p, tc.source, "X\\Dep")); got == "" {
				t.Errorf("X\\Dep, imported only in namespace A, adds no import in B")
			}
			own := completeAt(t, p, tc.source, "B\\Dep")
			if got := importText(own); got != "" {
				t.Errorf("B\\Dep in its own namespace adds %q", got)
			}
			if got := insertedName(own); got != "Dep($0)" {
				t.Errorf("B\\Dep inserted %q, want Dep($0)", got)
			}
		})
	}
}

func TestAutoImportGoesIntoTheNamespaceBlockAroundTheCursor(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///q/Other.php", "<?php\nnamespace Q;\nclass Other {}\n", symbols.SourceProject)
	p := NewProvider(idx, nil, "")
	for _, tc := range []struct {
		name, source, text string
		line               int
	}{
		{"unbraced, after the block's namespace", "<?php\nnamespace A;\nuse X\\Dep;\nclass Foo {}\n\nnamespace B;\n\nclass Bar { function g() { new Ot", "use Q\\Other;\n", 6},
		{"unbraced, after the block's last use", "<?php\nnamespace A;\nuse X\\Dep;\nclass Foo {}\n\nnamespace B;\nuse Y\\Thing;\nclass Bar { function g() { new Ot", "use Q\\Other;\n", 7},
		{"unbraced, after a group use spanning lines", "<?php\nnamespace B;\nuse Y\\{\n    Thing,\n    Other2\n};\nclass Bar { function g() { new Ot", "use Q\\Other;\n", 6},
		{"braced, inside the brace", "<?php\nnamespace A {\nuse X\\Dep;\n}\nnamespace B {\nclass Bar { function g() { new Ot", "use Q\\Other;\n\n", 5},
		{"braced, brace on its own line", "<?php\nnamespace A {\nuse X\\Dep;\n}\nnamespace B\n{\n\nclass Bar { function g() { new Ot", "use Q\\Other;\n", 6},
		{"global block", "<?php\nnamespace A {\nuse X\\Dep;\n}\nnamespace {\nclass Baz { function h() { new Ot", "use Q\\Other;\n\n", 5},
		{"no namespace, after declare", "<?php\ndeclare(strict_types=1);\n\nclass Baz { function h() { new Ot", "use Q\\Other;\n", 2},
		{"no namespace, after the open tag", "<p>\n<?php\nclass Baz { function h() { new Ot", "use Q\\Other;\n\n", 2},
		{"the word namespace in a comment", "<?php\n/*\n namespace Wrong\n*/\nnamespace B;\n\nclass Bar { function g() { new Ot", "use Q\\Other;\n", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := completeAt(t, p, tc.source, "Q\\Other")
			if len(item.AdditionalTextEdits) != 1 {
				t.Fatalf("edits = %#v, want one import", item.AdditionalTextEdits)
			}
			edit := item.AdditionalTextEdits[0]
			if edit.NewText != tc.text || edit.Range.Start != edit.Range.End || edit.Range.Start.Line != tc.line || edit.Range.Start.Character != 0 {
				t.Errorf("import %q at %+v, want %q at line %d", edit.NewText, edit.Range, tc.text, tc.line)
			}
		})
	}
}
