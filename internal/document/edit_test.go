package document

import (
	"strings"
	"testing"
)

func at(sl, sc, el, ec int) *Range {
	return &Range{Start: Position{Line: sl, Character: sc}, End: Position{Line: el, Character: ec}}
}

func TestApply(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		enc     Encoding
		changes []Change
		want    string
	}{
		{
			name:    "insert after non-ASCII text, UTF-16",
			text:    "<?php\n$a = 'Größe 😀 漢字'; $b;\n",
			enc:     UTF16,
			changes: []Change{{Range: at(1, 20, 1, 22), Text: "$total"}},
			want:    "<?php\n$a = 'Größe 😀 漢字'; $total;\n",
		},
		{
			name:    "insert after non-ASCII text, UTF-8",
			text:    "<?php\n$a = 'Größe 😀 漢字'; $b;\n",
			enc:     UTF8,
			changes: []Change{{Range: at(1, len("$a = 'Größe 😀 漢字'; "), 1, len("$a = 'Größe 😀 漢字'; $b")), Text: "$total"}},
			want:    "<?php\n$a = 'Größe 😀 漢字'; $total;\n",
		},
		{
			name:    "replace the emoji",
			text:    "$a = '😀';\n",
			enc:     UTF16,
			changes: []Change{{Range: at(0, 6, 0, 8), Text: "x"}},
			want:    "$a = 'x';\n",
		},
		{
			name:    "multi-line insert",
			text:    "<?php\nfoo();\n",
			enc:     UTF16,
			changes: []Change{{Range: at(1, 6, 1, 6), Text: "\nif (é) {\n    bar();\n}"}},
			want:    "<?php\nfoo();\nif (é) {\n    bar();\n}\n",
		},
		{
			name:    "delete across lines",
			text:    "<?php\n// é one\n// two\n// three\nend();\n",
			enc:     UTF16,
			changes: []Change{{Range: at(1, 5, 3, 3), Text: ""}},
			want:    "<?php\n// é three\nend();\n",
		},
		{
			name:    "CRLF line endings",
			text:    "<?php\r\n$é = 1;\r\n$b = 2;\r\n",
			enc:     UTF16,
			changes: []Change{{Range: at(2, 1, 2, 2), Text: "c"}, {Range: at(1, 7, 2, 0), Text: ""}},
			want:    "<?php\r\n$é = 1;$c = 2;\r\n",
		},
		{
			name:    "a column past a CRLF line's end stops before the CR",
			text:    "ab\r\ncd",
			enc:     UTF16,
			changes: []Change{{Range: at(0, 9, 0, 9), Text: "!"}},
			want:    "ab!\r\ncd",
		},
		{
			name: "several changes apply in order, each to the result of the last",
			text: "<?php\n$a = 'é';\n",
			enc:  UTF16,
			changes: []Change{
				{Range: at(1, 0, 1, 0), Text: "$z = 0;\n"},
				{Range: at(2, 1, 2, 2), Text: "b"},
				{Range: at(2, 6, 2, 7), Text: "😀"},
				{Range: at(2, 10, 2, 10), Text: " // ok"},
			},
			want: "<?php\n$z = 0;\n$b = '😀'; // ok\n",
		},
		{
			name:    "a change without a range replaces the text",
			text:    "<?php\nold();\n",
			enc:     UTF16,
			changes: []Change{{Range: at(1, 0, 1, 3), Text: "new"}, {Text: "<?php\nfresh();\n"}, {Range: at(1, 0, 1, 0), Text: "very"}},
			want:    "<?php\nveryfresh();\n",
		},
		{
			name:    "insert at the end of a text without a final newline",
			text:    "<?php\necho 'é'",
			enc:     UTF16,
			changes: []Change{{Range: at(1, 8, 1, 8), Text: ";\n"}},
			want:    "<?php\necho 'é';\n",
		},
		{
			name:    "an end at the start of the line after the last clamps to the end of the text",
			text:    "a\nb",
			enc:     UTF16,
			changes: []Change{{Range: at(0, 1, 2, 0), Text: ""}},
			want:    "a",
		},
		{
			name:    "a range past the last line inserts at the end",
			text:    "<?php\né\n",
			enc:     UTF16,
			changes: []Change{{Range: at(9, 4, 12, 0), Text: "end();"}},
			want:    "<?php\né\nend();",
		},
		{
			name: "one change per cursor, bottom to top, as VS Code sends them",
			text: "<?php\n$a = 'é';\n$b = 'é';\n$c = 'é';\n",
			enc:  UTF16,
			changes: []Change{
				{Range: at(3, 0, 3, 2), Text: "$zz"},
				{Range: at(2, 0, 2, 2), Text: ""},
				{Range: at(1, 0, 1, 2), Text: "$yy"},
			},
			want: "<?php\n$yy = 'é';\n = 'é';\n$zz = 'é';\n",
		},
		{
			name: "one change per cursor, top to bottom, growing and shrinking the text",
			text: "<?php\n$a = 'é';\n$b = 'é';\n$c = 'é';\n",
			enc:  UTF16,
			changes: []Change{
				{Range: at(1, 6, 1, 7), Text: "😀😀\n"},
				{Range: at(3, 0, 4, 0), Text: ""},
				{Range: at(3, 6, 3, 7), Text: ""},
				{Range: at(0, 5, 0, 5), Text: " // top"},
			},
			want: "<?php // top\n$a = '😀😀\n';\n$c = '';\n",
		},
		{
			name:    "a lone CR ends a line",
			text:    "<?php\r$a = 'é';\r$b;",
			enc:     UTF16,
			changes: []Change{{Range: at(2, 0, 2, 0), Text: "x"}},
			want:    "<?php\r$a = 'é';\rx$b;",
		},
		{
			name:    "an edit spanning a lone CR",
			text:    "<?php\r$a = 'é';\r$b;",
			enc:     UTF16,
			changes: []Change{{Range: at(1, 7, 2, 1), Text: ""}},
			want:    "<?php\r$a = 'éb;",
		},
		{
			name:    "a column past a line that ends with a lone CR stops before it",
			text:    "a\rb",
			enc:     UTF16,
			changes: []Change{{Range: at(0, 5, 0, 5), Text: "!"}},
			want:    "a!\rb",
		},
		{
			name:    "an insert that turns a lone CR into a CRLF",
			text:    "a\rb",
			enc:     UTF16,
			changes: []Change{{Range: at(1, 0, 1, 0), Text: "\n"}, {Range: at(1, 0, 1, 0), Text: "x"}},
			want:    "a\r\nxb",
		},
		{
			name: "cursors bottom to top over mixed line endings",
			text: "<?php\r$a;\r\n$b;\n$c;\r\r$d;",
			enc:  UTF16,
			changes: []Change{
				{Range: at(5, 0, 5, 1), Text: "@"},
				{Range: at(3, 0, 3, 1), Text: "@"},
				{Range: at(2, 0, 2, 1), Text: "@"},
				{Range: at(1, 0, 1, 1), Text: "@"},
			},
			want: "<?php\r@a;\r\n@b;\n@c;\r\r@d;",
		},
	}
	for _, c := range cases {
		got, err := Apply(c.text, c.enc, c.changes)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestApplyRejectsChangesThatDoNotFit(t *testing.T) {
	cases := []struct {
		name   string
		change Change
		want   string
	}{
		{"end before start", Change{Range: at(1, 3, 1, 1)}, "before its start"},
		{"negative start", Change{Range: at(-1, 0, 0, 0)}, "range start: position -1:0 is negative"},
		{"negative end", Change{Range: at(0, 0, 0, -1)}, "range end: position 0:-1 is negative"},
	}
	for _, c := range cases {
		_, err := Apply("<?php\nabc\n", UTF16, []Change{{Range: at(0, 0, 0, 0), Text: ""}, c.change})
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "change 2 of 2") {
			t.Errorf("%s: error %v, want one naming change 2 and %q", c.name, err, c.want)
		}
	}
}
