package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/rule"
)

func TestLayoutWrap(t *testing.T) {
	l := PHPLayout("<?php\n")
	if got := l.Wrap(PHPComment, "@tusk-ignore x"); got != "// @tusk-ignore x" {
		t.Errorf("got %q", got)
	}
	if got := l.Wrap(PHPBlockComment, "@tusk-ignore x"); got != "/* @tusk-ignore x */" {
		t.Errorf("got %q", got)
	}
}

// A template layout knows PHP; the template's own comments, PHP regions,
// constructs and delimiters come from its language's package.
func TestTemplateLayout(t *testing.T) {
	// <%# ... %> comments, <?= ... ?> PHP: a made-up template language.
	source := "<div>\n<%# @tusk-ignore unknown-function %>\n<?= nope() ?>\n<?=\n    // @tusk-ignore undefined-variable\n    $a\n?>\n</div>\n"
	at := func(s string) int { return strings.Index(source, s) }
	tpl := Template{
		Comments:     []Span{{at("<%#"), at("%>") + 2}},
		PHP:          []Span{{at("nope()"), at(" ?>")}, {at("\n    //"), at("\n?>")}},
		CommentOpen:  "<%#",
		CommentClose: "%>",
	}
	l := TemplateLayout(source, tpl)
	if got := l.Wrap(TemplateComment, "@tusk-ignore x"); got != "<%# @tusk-ignore x %>" {
		t.Errorf("template comment: got %q", got)
	}
	for line, want := range map[int]struct {
		style CommentStyle
		ok    bool
	}{0: {TemplateComment, true}, 4: {PHPComment, true}, 7: {TemplateComment, true}} {
		start, _ := l.LineStart(line)
		if style, ok := l.StyleAt(start); style != want.style || ok != want.ok {
			t.Errorf("line %d: %v %v, want %v", line, style, ok, want)
		}
	}
	s := l.Suppressions()
	for _, c := range []struct {
		line int
		code string
		want bool
	}{{2, "unknown-function", true}, {5, "undefined-variable", true}, {3, "unknown-function", false}} {
		if got := s.Suppresses(c.line, string(rule.SourceTusk), c.code); got != c.want {
			t.Errorf("line %d %s: got %v, want %v", c.line, c.code, got, c.want)
		}
	}
}

func TestLayoutMarkerLine(t *testing.T) {
	source := "<?php\nclass Shop\n{\n    /** @var int */\n    #[A([1, 2])] #[B]\n\n    public $total;\n    $a = 1; // trailing\n    #[Unclosed(\n"
	l := PHPLayout(source)
	for line, want := range map[int]int{6: 3, 2: 2, 7: 7, 8: 8} {
		if got := l.MarkerLine(line); got != want {
			t.Errorf("marker for line %d goes on line %d, want %d", line, got, want)
		}
	}
}

func TestLayoutStyleAt(t *testing.T) {
	source := "<p>html</p>\n<?php\n$s = \"a\nb\";\n?>\n<p>again</p>\n"
	l := PHPLayout(source)
	cases := []struct {
		line int
		ok   bool
	}{{0, false}, {2, true}, {3, false}, {4, true}, {5, false}}
	for _, c := range cases {
		start, _ := l.LineStart(c.line)
		if style, ok := l.StyleAt(start); ok != c.ok || (ok && style != PHPComment) {
			t.Errorf("line %d: %v %v, want %v", c.line, style, ok, c.ok)
		}
	}
}

func TestLayoutLinesAndWhitespace(t *testing.T) {
	l := PHPLayout("<?php\r\n\t$a = 1;\r\n")
	if l.LineEnding() != "\r\n" || PHPLayout("<?php\n").LineEnding() != "\n" {
		t.Error("the file's own line ending is kept")
	}
	if l.Indent(1) != "\t" || l.Indent(0) != "" {
		t.Errorf("indent %q", l.Indent(1))
	}
	if _, ok := l.LineStart(-1); ok {
		t.Error("no line before the first")
	}
	if _, ok := l.LineStart(3); ok {
		t.Error("no line past the last")
	}
	if l.Line(8) != 1 {
		t.Errorf("offset 8 is on line %d", l.Line(8))
	}
}

func TestLayoutMarkerLineStopsBeyondOneBlankLine(t *testing.T) {
	source := "<?php\n/** @var int */\n\n\n$a = 1;\n/** @var int */\n\n$b = 1;\n"
	l := PHPLayout(source)
	for line, want := range map[int]int{4: 4, 7: 5} {
		if got := l.MarkerLine(line); got != want {
			t.Errorf("marker for line %d goes on line %d, want %d", line, got, want)
		}
	}
}

func TestLayoutTrailingCommentAt(t *testing.T) {
	source := "<?php\n$a = 'é'; // note\n$b = 1;\n$c = \"x\ny\";\n?>\n<p><?php $d ?></p>\n/* a */ // b\n"
	l := PHPLayout(source)
	cases := []struct {
		line      int
		ok        bool
		character int
		style     CommentStyle
	}{
		{1, true, len("$a = 'é'; "), PHPBlockComment},
		{2, true, 7, PHPComment},
		{3, false, 0, 0},
		{4, true, 3, PHPComment},
		{5, false, 0, 0},
		{6, false, 0, 0},
		{7, false, 0, 0},
	}
	for _, c := range cases {
		offset, style, ok := l.TrailingCommentAt(c.line)
		if ok != c.ok || (ok && (l.Character(offset) != c.character || style != c.style)) {
			t.Errorf("line %d: offset %d style %v ok %v, want character %d style %v ok %v", c.line, offset, style, ok, c.character, c.style, c.ok)
		}
	}
	if _, _, ok := PHPLayout("<?php\n/* a */ // b\n").TrailingCommentAt(1); ok {
		t.Error("a line of comments has no code to ignore")
	}
	if _, _, ok := l.TrailingCommentAt(99); ok {
		t.Error("no line past the end")
	}
}
