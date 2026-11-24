package checks

import "testing"

func TestCommentStyleWrap(t *testing.T) {
	if got := PHPComment.Wrap("@tusk-ignore x"); got != "// @tusk-ignore x" {
		t.Errorf("got %q", got)
	}
	if got := BladeComment.Wrap("@tusk-ignore x"); got != "{{-- @tusk-ignore x --}}" {
		t.Errorf("got %q", got)
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
	view := BladeLayout("<div>\n@php\n    $s = 'a\nb';\n@endphp\n</div>\n")
	for line, want := range map[int]struct {
		style CommentStyle
		ok    bool
	}{0: {BladeComment, true}, 2: {PHPComment, true}, 3: {PHPComment, false}, 5: {BladeComment, true}} {
		start, _ := view.LineStart(line)
		if style, ok := view.StyleAt(start); style != want.style || ok != want.ok {
			t.Errorf("blade line %d: %v %v, want %v", line, style, ok, want)
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
