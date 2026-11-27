package checks

import (
	"reflect"
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
$i->missing(); // TODO: drop @tusk-ignore
/** Legacy, see @tusk-ignore */
$j->missing();
/**
 * Read by an include.
 * @tusk-ignore unknown-member
 */
$k->missing();
`
	checkSuppressions(t, PHPSuppressions(source, nil), []suppressionCase{
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
		{14, tusk, "unknown-member", false},
		{16, tusk, "unknown-member", false},
		{21, tusk, "unknown-member", true},
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
	checkSuppressions(t, PHPSuppressions(source, nil), []suppressionCase{
		{2, phpstan, "method.notFound", true},
		{2, tusk, "unknown-member", true},
		{2, tusk, "undefined-variable", false},
		{3, phpstan, "argument.type", true},
		{3, tusk, "undefined-variable", true},
		{3, tusk, "unknown-member", false},
		{5, tusk, "unknown-member", true},
		{5, phpstan, "method.notFound", true},
		{5, pint, "braces", false},
		{6, pint, "braces", false},
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
	if got := PHPSuppressions("<?php\n// nothing to see\n$a = 1;\n", nil); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}

func TestPHPSuppressionsFromSharedTokens(t *testing.T) {
	source := "<?php\n/**\n * @return int\n */\n// @tusk-ignore unknown-member\n#[\\Attr]\nfunction f() { return $a->b(); } // @phpstan-ignore variable.undefined\n"
	shared := &Shared{}
	shared.parse(source)
	if got, want := PHPSuppressions(source, shared), PHPSuppressions(source, nil); !reflect.DeepEqual(got, want) {
		t.Errorf("from the shared parse: got %#v, want %#v", got, want)
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

func TestOwnLineMarkersCoverDocblocksAttributesAndTheCodeLine(t *testing.T) {
	source := `<?php
class Shop
{
    // @tusk-ignore unknown-class
    // @tusk-ignore unknown-member
    /**
     * @return Missing
     */
    #[Route('/cart', methods: ['GET'])]
    #[Deprecated]
    public function cart() {}

    #[Route(
        // @tusk-ignore unknown-class
        Missing::class,
    )]
    public function pay() {}
}
`
	checkSuppressions(t, PHPSuppressions(source, nil), []suppressionCase{
		{6, tusk, "unknown-class", true},
		{8, tusk, "unknown-class", true},
		{9, tusk, "unknown-member", true},
		{10, tusk, "unknown-class", true},
		{10, tusk, "unknown-member", true},
		{11, tusk, "unknown-class", false},
		{12, tusk, "unknown-class", false},
		{14, tusk, "unknown-class", true},
		{16, tusk, "unknown-class", false},
	})
}

func TestOwnLineMarkersReachAcrossAtMostOneBlankLine(t *testing.T) {
	source := `<?php
// @tusk-ignore unused-variable

$a = 1;
// @tusk-ignore unused-variable


$b = 1;
// @tusk-ignore unused-variable

// unrelated note

$c = 1;
`
	checkSuppressions(t, PHPSuppressions(source, nil), []suppressionCase{
		{3, tusk, "unused-variable", true},
		{7, tusk, "unused-variable", false},
		{10, tusk, "unused-variable", false},
		{12, tusk, "unused-variable", false},
	})
}

func TestPHPStanMarkersCoverTheLinesPHPStanIgnores(t *testing.T) {
	source := `<?php
class Fixture
{
    // @phpstan-ignore method.notFound
    /**
     * @return int
     */
    public function aboveDocblock(): int { return $this->nope1(); }
    public function trailing(): int { return $this->nope2(); } // @phpstan-ignore method.notFound
    public function beforeComment(): int { return $this->nope3(); } /* @phpstan-ignore method.notFound */ // keep this note
    // @phpstan-ignore attribute.notFound

    #[NoSuchAttr]
    public function blankGap(): void {}
    // @phpstan-ignore attribute.notFound
    #[NoSuchAttr2]
    // @phpstan-ignore-next-line
    // note
    public function nextLinePastComment(): int { return $this->nope6(); }
    /**
     * @phpstan-ignore-next-line
     */
    public function docNextLine(): int { return $this->nope7(); }
}
`
	checkSuppressions(t, PHPSuppressions(source, nil), []suppressionCase{
		{7, phpstan, "method.notFound", false},
		{7, tusk, "unknown-member", false},
		{8, phpstan, "method.notFound", true},
		{9, phpstan, "method.notFound", true},
		{12, phpstan, "attribute.notFound", false},
		{15, phpstan, "attribute.notFound", true},
		{18, phpstan, "method.notFound", false},
		{18, tusk, "unknown-member", false},
		{22, phpstan, "method.notFound", true},
		{22, tusk, "unknown-member", true},
	})
}

func TestMarkerCodesStopAtPunctuationAndReasons(t *testing.T) {
	source := `<?php
$a = 1; // @tusk-ignore unused-variable.
$b = 1; // @tusk-ignore unused-variable: read by an include
$c->d(); // @phpstan-ignore method.notFound (magic), variable.undefined (legacy (old))
$e->f(); // @phpstan-ignore method.notFound (unclosed
$g->h(); // @tusk-ignore pint:single_quote
$i->j(); // @tusk-ignore: legacy
`
	checkSuppressions(t, PHPSuppressions(source, nil), []suppressionCase{
		{1, tusk, "unused-variable", true},
		{2, tusk, "unused-variable", true},
		{3, tusk, "unknown-member", true},
		{3, tusk, "undefined-variable", true},
		{4, tusk, "unknown-member", true},
		{5, pint, rule.PintCode([]string{"binary_operator_spaces", "single_quote"}), true},
		{5, pint, "braces", false},
		{6, tusk, "unknown-member", true},
	})
}
