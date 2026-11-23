package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/rule"
)

const (
	tusk    = string(rule.SourceTusk)
	phpstan = string(rule.SourcePHPStan)
	pint    = string(rule.SourcePint)
)

type suppressionCase struct {
	line   int
	source string
	code   string
	want   bool
}

func checkSuppressions(t *testing.T, s Suppressions, cases []suppressionCase) {
	t.Helper()
	for _, c := range cases {
		if got := s.Suppresses(c.line, c.source, c.code); got != c.want {
			t.Errorf("line %d %s:%s: got %v, want %v", c.line, c.source, c.code, got, c.want)
		}
	}
}

func TestTuskIgnoreComments(t *testing.T) {
	source := `<?php
// @tusk-ignore unknown-member
$a->missing();
$b->missing(); // @tusk-ignore unknown-member, undefined-variable (legacy)
/* @tusk-ignore */ $c->missing();
/**
 * @tusk-ignore phpstan:argument.type
 */
foo($d);
$e = '// @tusk-ignore';
// @tusk-ignore-foo
$f = 1; # @tusk-ignore unused-variable because it is read by an include
$g->missing(); // @tusk-ignore unknwon-member
$h(); // @tusk-ignore pint
`
	checkSuppressions(t, PHPSuppressions(source), []suppressionCase{
		{1, tusk, "unknown-member", false},
		{2, tusk, "unknown-member", true},
		{2, tusk, "undefined-variable", false},
		{3, tusk, "unknown-member", true},
		{3, tusk, "undefined-variable", true},
		{3, tusk, "unused-variable", false},
		{4, tusk, "unknown-member", true},
		{4, phpstan, "method.notFound", true},
		{8, phpstan, "argument.type", true},
		{8, tusk, "unknown-function", false},
		{9, tusk, "unused-variable", false},
		{10, tusk, "unused-variable", false},
		{11, tusk, "unused-variable", true},
		{12, tusk, "unknown-member", false},
		{13, pint, "braces", true},
		{13, phpstan, "braces", false},
	})
}

func TestPHPStanIgnoreComments(t *testing.T) {
	source := `<?php
// @phpstan-ignore method.notFound
$a->missing();
$b->missing(); // @phpstan-ignore variable.undefined, argument.type (legacy)
/** @phpstan-ignore-next-line */
$c->missing();
$d->missing(); // @phpstan-ignore-line
// @phpstan-ignore
$e->missing();
`
	checkSuppressions(t, PHPSuppressions(source), []suppressionCase{
		{2, phpstan, "method.notFound", true},
		{2, tusk, "unknown-member", true},
		{2, tusk, "undefined-variable", false},
		{3, phpstan, "argument.type", true},
		{3, tusk, "undefined-variable", true},
		{3, tusk, "unknown-member", false},
		{5, tusk, "unknown-member", true},
		{5, pint, "braces", true},
		{6, tusk, "unknown-member", true},
		{8, tusk, "unknown-member", false},
		{8, phpstan, "method.notFound", false},
	})
}

func TestPHPStanIdentifiersMapToDeclaredCodes(t *testing.T) {
	for identifier, code := range phpstanIdentifierCodes {
		if _, err := rule.ParseCode(string(code)); err != nil {
			t.Errorf("%s maps to %q: %v", identifier, code, err)
		}
	}
}

func TestPHPSuppressionsWithoutMarker(t *testing.T) {
	if got := PHPSuppressions("<?php\n// nothing to see\n$a = 1;\n"); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
	if got := CommentSuppressions("<p>{{-- plain --}}</p>", []CommentSpan{{3, 18}}); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}

func TestSuppressionComments(t *testing.T) {
	if got := SuppressionComment(rule.NativeTarget(rule.UnknownMember)); got != "@tusk-ignore unknown-member" {
		t.Errorf("got %q", got)
	}
	if got := PHPStanSuppressionComment("argument.type"); got != "@phpstan-ignore argument.type" {
		t.Errorf("got %q", got)
	}
}

func TestCommentSuppressionsForTemplates(t *testing.T) {
	source := "<div>\n{{-- @tusk-ignore unknown-view --}}\n@include('missing')\n{{ $a }} {{-- @tusk-ignore undefined-variable --}}\n{{-- a note --}}\n</div>\n"
	var spans []CommentSpan
	for from := 0; ; {
		start := strings.Index(source[from:], "{{--")
		if start < 0 {
			break
		}
		start += from
		end := start + strings.Index(source[start:], "--}}") + len("--}}")
		spans = append(spans, CommentSpan{start, end})
		from = end
	}
	checkSuppressions(t, CommentSuppressions(source, spans), []suppressionCase{
		{2, tusk, "unknown-view", true},
		{3, tusk, "undefined-variable", true},
		{3, tusk, "unknown-view", false},
		{4, tusk, "undefined-variable", false},
	})
}
