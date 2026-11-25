package checks

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
)

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
	view := BladeLayout("<div>\n@php\n    $s = 'a\nb';\n@endphp\n</div>\n", nil)
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

func TestLayoutMarkerLineStopsBeyondOneBlankLine(t *testing.T) {
	source := "<?php\n/** @var int */\n\n\n$a = 1;\n/** @var int */\n\n$b = 1;\n"
	l := PHPLayout(source)
	for line, want := range map[int]int{4: 4, 7: 5} {
		if got := l.MarkerLine(line); got != want {
			t.Errorf("marker for line %d goes on line %d, want %d", line, got, want)
		}
	}
}

func TestBladeLayoutMarkerLineSkipsAttributesInPHPBlocks(t *testing.T) {
	l := BladeLayout("<div>\n@php\n    #[A]\n    function f() {}\n@endphp\n</div>\n", nil)
	if got := l.MarkerLine(3); got != 2 {
		t.Errorf("marker goes above the attribute on line 2, got %d", got)
	}
}

func TestBladeLayoutMarkerLineGoesAboveAMultiLineDirective(t *testing.T) {
	l := BladeLayout("<div>\n    <p>@include('row', [\n        'total' => missing(),\n        'tax' => {{ 1 }},\n    ])</p>\n{{ $a }}\n</div>\n", nil)
	for line, want := range map[int]int{1: 1, 2: 1, 3: 1, 4: 1, 5: 5} {
		if got := l.MarkerLine(line); got != want {
			t.Errorf("marker for line %d goes on line %d, want %d", line, got, want)
		}
	}
}

func TestBladeLayoutMarkerLineGoesAboveAMultiLineCustomDirective(t *testing.T) {
	source := "<div>\n    @admin([\n        'role' => missing(),\n    ])\n    x\n    @endadmin\n</div>\n"
	custom := &blade.Custom{Conditionals: map[string]bool{"admin": true}}
	l := BladeLayout(source, custom)
	for line, want := range map[int]int{1: 1, 2: 1, 3: 1, 4: 4} {
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
