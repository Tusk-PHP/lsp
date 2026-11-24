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
		"dynamic fetch":              "<?php\nclass A {\n    private const X = 1;\n    public function f(string $n) { return self::{$n}; }\n}\n",
		"constant()":                 "<?php\nclass A {\n    private const X = 1;\n    public function f() { return constant('self::X'); }\n}\n",
		"class attribute":            "<?php\n#[AsCommand(name: self::NAME)]\nfinal class FooCommand {\n    private const NAME = 'app:foo';\n}\n",
		"multi-line class attribute": "<?php\nuse Doctrine\\ORM\\Mapping as ORM;\n#[ORM\\Entity]\n#[ORM\\Table(\n    name: self::TABLE,\n)]\nclass Post {\n    private const TABLE = 'posts';\n}\n",
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

func TestUnusedPrivateConstantReadsTokensOnlyForClassesWithPrivateConstants(t *testing.T) {
	src := "<?php\nclass A {\n    public const X = 1;\n    private int $p = 0;\n}\n"
	cls := parser.ParseFile(src).Classes[0]
	tokens := func() []parser.Token {
		t.Fatal("tokens fetched for a class without private constants")
		return nil
	}
	if got := checkUnusedPrivateConstants(cls, tokens, strings.Split(src, "\n"), nil); got != nil {
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
