package checks

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func privateConstantFindings(t *testing.T, src string, idx *symbols.Index) []string {
	t.Helper()
	var got []string
	for _, f := range (&UnusedPrivateRule{}).Check(parser.ParseFile(src), src, idx) {
		if f.Code == rule.UnusedPrivateConstant {
			if f.Severity != rule.SeverityHint || !slices.Equal(f.Tags, []Tag{TagUnnecessary}) {
				t.Errorf("finding %+v", f)
			}
			got = append(got, f.Message)
		}
	}
	return got
}

func TestUnusedPrivateConstant(t *testing.T) {
	src := `<?php
class Config {
    private const UNUSED = 1;
    private const VIA_SELF = 2;
    private const VIA_STATIC = 3;
    private const IN_OTHER = 4;
    private const ONLY_HERE = self::IN_OTHER;
    public const PUBLIC_ONE = 5;
    protected const PROTECTED_ONE = 6;
    public function read(): int { return self::VIA_SELF + static::VIA_STATIC; }
}
`
	got := privateConstantFindings(t, src, nil)
	want := []string{"Private constant 'Config::UNUSED' is never used", "Private constant 'Config::ONLY_HERE' is never used"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUnusedPrivateConstantStaysQuietWhenUnsure(t *testing.T) {
	for name, src := range map[string]string{
		"dynamic fetch":                        "<?php\nclass A {\n    private const X = 1;\n    public function f(string $n) { return self::{$n}; }\n}\n",
		"constant()":                           "<?php\nclass A {\n    private const X = 1;\n    public function f() { return constant('self::X'); }\n}\n",
		"constant() of a computed name":        "<?php\nclass A {\n    private const X = 1;\n    public function f(string $n) { return constant('self::' . $n); }\n}\n",
		"defined()":                            "<?php\nclass A {\n    private const X = 1;\n    public function f() { return defined('self::X'); }\n}\n",
		"constant() of an interpolated string": "<?php\nclass A {\n    private const X = 1;\n    public function f(string $n) { return constant(\"self::{$n}\"); }\n}\n",
		"constant() of a nowdoc":               "<?php\nclass A {\n    private const X = 1;\n    public function f() { return constant(<<<'N'\nself::X\nN); }\n}\n",
		"defined() of a variable":              "<?php\nclass A {\n    private const X = 1;\n    public function f(string $n) { return defined($n); }\n}\n",
		"getConstants()":                       "<?php\nclass A {\n    private const X = 1;\n    public function f() { return (new \\ReflectionClass($this))->getConstants(); }\n}\n",
		"getConstant()":                        "<?php\nclass A {\n    private const X = 1;\n    public function f(string $n) { return (new \\ReflectionClass(self::class))->getConstant($n); }\n}\n",
		"ReflectionClassConstant":              "<?php\nclass A {\n    private const X = 1;\n    public function f(string $n) { return new \\ReflectionClassConstant(self::class, $n); }\n}\n",
		"class attribute":                      "<?php\n#[AsCommand(name: self::NAME)]\nfinal class FooCommand {\n    private const NAME = 'app:foo';\n}\n",
		"multi-line class attribute":           "<?php\nuse Doctrine\\ORM\\Mapping as ORM;\n#[ORM\\Entity]\n#[ORM\\Table(\n    name: self::TABLE,\n)]\nclass Post {\n    private const TABLE = 'posts';\n}\n",
	} {
		if got := privateConstantFindings(t, src, nil); len(got) != 0 {
			t.Errorf("%s: %v", name, got)
		}
	}
	idx := symbols.NewIndex()
	idx.IndexFile("file:///T.php", "<?php\ntrait UsesX { public function g() { return self::YZ + self::X; } }\n")
	src := "<?php\nclass B {\n    use UsesX;\n    private const X = 1;\n    private const Y = 2;\n}\n"
	idx.IndexFile("file:///B.php", src)
	if got := privateConstantFindings(t, src, idx); !slices.Equal(got, []string{"Private constant 'B::Y' is never used"}) {
		t.Errorf("a trait's use counts: %v", got)
	}
}

func TestUnusedPrivateConstantChecksLiteralLookupsByName(t *testing.T) {
	src := `<?php
class A {
    private const READ = 1;
    private const UNREAD = 2;
    private bool $defined = false;
    public function f() { return $this->defined || defined('APP_DEBUG') && constant('self::READ'); }
}
`
	if got := privateConstantFindings(t, src, nil); !slices.Equal(got, []string{"Private constant 'A::UNREAD' is never used"}) {
		t.Errorf("got %v", got)
	}
}

func TestUnusedPrivateConstantReadsInterpolatedExpressions(t *testing.T) {
	src := `<?php
class A {
    private const IN_STRING = 'a';
    private const IN_HEREDOC_SUBSCRIPT = 'b';
    private const IN_HEREDOC_CALL = 'c';
    private const VIA_STATIC = 'd';
    private const VIA_CLASS_NAME = 'e';
    private const IN_DOLLAR_BRACE = 'f';
    private const IN_NOWDOC = 'g';
    private const IN_PLAIN_STRING = 'h';
    private const IN_SINGLE_QUOTES = 'i';
    private const IN_HEREDOC_TEXT = 'j';
    private const AFTER_ESCAPED_BRACE = 'k';
    public function f(array $a): string {
        $s = "v={$a[self::IN_STRING]} {$a[static::VIA_STATIC]} {$a[A::VIA_CLASS_NAME]} ${a[self::IN_DOLLAR_BRACE]}";
        $s .= <<<EOT
            {$a[self::IN_HEREDOC_SUBSCRIPT]} {$this->g(self::IN_HEREDOC_CALL)} self::IN_HEREDOC_TEXT
            EOT;
        $s .= <<<'EOT'
            {$a[self::IN_NOWDOC]}
            EOT;
        $s .= "self::IN_PLAIN_STRING \{$a}[self::AFTER_ESCAPED_BRACE]" . '{$a[self::IN_SINGLE_QUOTES]}';
        return $s;
    }
    private function g(string $k): string { return $k; }
}
`
	want := []string{
		"Private constant 'A::IN_NOWDOC' is never used",
		"Private constant 'A::IN_PLAIN_STRING' is never used",
		"Private constant 'A::IN_SINGLE_QUOTES' is never used",
		"Private constant 'A::IN_HEREDOC_TEXT' is never used",
		"Private constant 'A::AFTER_ESCAPED_BRACE' is never used",
	}
	if got := privateConstantFindings(t, src, nil); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUnusedPrivateReadsTokensOnlyForClassesWithPrivateMembers(t *testing.T) {
	src := "<?php\nclass A {\n    public const X = 1;\n    protected int $p = 0;\n    public function f(): void {}\n}\n"
	cls := parser.ParseFile(src).Classes[0]
	tokens := func() []parser.Token {
		t.Fatal("tokens fetched for a class without private members")
		return nil
	}
	if got := checkUnusedPrivateMembers(cls, "", tokens, strings.Split(src, "\n"), nil); got != nil {
		t.Errorf("got %v", got)
	}
}

func TestUnusedPrivateRuleDeclaresEveryCodeItReports(t *testing.T) {
	src := `<?php
class Lonely {
    private const C = 1;
    private int $p = 0;
    private function m(): void {}
}
`
	r := &UnusedPrivateRule{}
	declared := r.Codes()
	seen := map[rule.Code]bool{}
	for _, f := range r.Check(parser.ParseFile(src), src, nil) {
		if !slices.Contains(declared, f.Code) {
			t.Errorf("reported %s without declaring it in %v", f.Code, declared)
		}
		seen[f.Code] = true
	}
	if len(seen) != len(declared) {
		t.Errorf("declared %v, reported %v", declared, seen)
	}
}
