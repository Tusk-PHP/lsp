package diagnostics

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

const tusk = string(rule.SourceTusk)

type suppressionCase struct {
	line   int
	source string
	code   string
	want   bool
}

func checkSuppressions(t *testing.T, s checks.Suppressions, cases []suppressionCase) {
	t.Helper()
	for _, c := range cases {
		if got := s.Suppresses(c.line, c.source, c.code); got != c.want {
			t.Errorf("line %d %s:%s: got %v, want %v", c.line, c.source, c.code, got, c.want)
		}
	}
}

func TestBladeSuppressionsWithoutMarker(t *testing.T) {
	if got := bladeSuppressions("<p>{{-- plain --}}</p>", nil); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}

func TestBladeLayoutStyleAndComments(t *testing.T) {
	view := BladeLayout("<div>\n@php\n    $s = 'a\nb';\n@endphp\n</div>\n", nil)
	for line, want := range map[int]struct {
		style checks.CommentStyle
		ok    bool
	}{0: {checks.TemplateComment, true}, 2: {checks.PHPComment, true}, 3: {checks.PHPComment, false}, 5: {checks.TemplateComment, true}} {
		start, _ := view.LineStart(line)
		if style, ok := view.StyleAt(start); style != want.style || ok != want.ok {
			t.Errorf("blade line %d: %v %v, want %v", line, style, ok, want)
		}
	}
	if got := view.Wrap(checks.TemplateComment, "@tusk-ignore x"); got != "{{-- @tusk-ignore x --}}" {
		t.Errorf("got %q", got)
	}
}

func TestBladeSuppressions(t *testing.T) {
	source := "<div>\n{{-- @tusk-ignore unknown-view --}}\n@include('missing')\n{{ $a }} {{-- @tusk-ignore undefined-variable --}}\n{{-- a note --}}\n@php\n    // @tusk-ignore unknown-function\n    nope();\n@endphp\n{{-- @tusk-ignore unknown-member--}}\n{{ $b->c }}\n</div>\n"
	checkSuppressions(t, bladeSuppressions(source, nil), []suppressionCase{
		{2, tusk, "unknown-view", true},
		{3, tusk, "undefined-variable", true},
		{3, tusk, "unknown-view", false},
		{4, tusk, "undefined-variable", false},
		{7, tusk, "unknown-function", true},
		{10, tusk, "unknown-member", true},
	})
}

func TestBladeMarkersCoverMultiLineDirectivesAndEchoes(t *testing.T) {
	source := "<div>\n{{-- @tusk-ignore unknown-function --}}\n@include('row', [\n    'total' => missing(),\n])\n<p>{{ $a }}</p>\n{{ format(\n    $b,\n) }} {{-- @tusk-ignore undefined-variable --}}\n@include('row', [\n    'total' => missing(),\n])\n</div>\n"
	checkSuppressions(t, bladeSuppressions(source, nil), []suppressionCase{
		{2, tusk, "unknown-function", true},
		{3, tusk, "unknown-function", true},
		{4, tusk, "unknown-function", true},
		{5, tusk, "undefined-variable", false},
		{6, tusk, "undefined-variable", true},
		{7, tusk, "undefined-variable", true},
		{8, tusk, "undefined-variable", true},
		{9, tusk, "unknown-function", false},
		{10, tusk, "unknown-function", false},
	})
}

func TestBladeMarkersCoverMultiLineCustomDirectives(t *testing.T) {
	source := "<div>\n{{-- @tusk-ignore unknown-function --}}\n@admin([\n    'role' => missing(),\n])\nx\n@endadmin\n{{-- @tusk-ignore unknown-function --}}\n@money(\n    missing(),\n)\n</div>\n"
	custom := &blade.Custom{Directives: map[string]bool{"money": true}, Conditionals: map[string]bool{"admin": true}}
	checkSuppressions(t, bladeSuppressions(source, custom), []suppressionCase{
		{2, tusk, "unknown-function", true},
		{3, tusk, "unknown-function", true},
		{4, tusk, "unknown-function", true},
		{5, tusk, "unknown-function", false},
		{9, tusk, "unknown-function", true},
		{10, tusk, "unknown-function", true},
	})
	checkSuppressions(t, bladeSuppressions(source, nil), []suppressionCase{
		{3, tusk, "unknown-function", false},
	})
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
