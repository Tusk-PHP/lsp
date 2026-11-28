package parser

import (
	"slices"
	"testing"
)

func interpolatedValues(lit string) []string {
	var values []string
	for _, tok := range InterpolatedTokens(Token{Kind: TokenStringLiteral, Value: lit}) {
		values = append(values, tok.Value)
	}
	return values
}

func TestInterpolatedTokensAreTheCodeInsideAString(t *testing.T) {
	for lit, want := range map[string][]string{
		`"v={$a[self::X]} and ${b} ${c[self::Y]} ${$d}"`: {"$a", "[", "self", "::", "X", "]", "[", "self", "::", "Y", "]", "$d"},
		`"{$f(fn() => ['}' => 1])} {$g->h("{")}"`:        {"$f", "(", "fn", "(", ")", "=>", "[", "'}'", "=>", "1", "]", ")", "$g", "->", "h", "(", `"{"`, ")"},
		`"$a->b $c?->d $e[k] $f[$g] $h->"`:               {"$a", "->", "b", "$c", "?", "->", "d", "$e", "$f", "$g", "$h"},
		`"{$a[$m["{$n}"]]}"`:                             {"$a", "[", "$m", "[", `"{$n}"`, "]", "]"},
		"<<<EOT\n{$this->g(self::X)}\nEOT":               {"$this", "->", "g", "(", "self", "::", "X", ")"},
		"<<<\"EOT\"\n{$a}\nEOT":                          {"$a"},
		"<<<'EOT'\n{$a[self::X]}\nEOT":                   nil,
		`"a list of names"`:                              nil,
		`'{$a[self::X]}'`:                                nil,
		`"self::X {\$a} \{$b} \$c"`:                      {"$b"},
		`"{$a[self::X]"`:                                 {"$a", "[", "self", "::", "X", "]", `"`},
	} {
		if got := interpolatedValues(lit); !slices.Equal(got, want) {
			t.Errorf("InterpolatedTokens(%s) = %q, want %q", lit, got, want)
		}
	}
}

func TestInterpolatedTokensOnlyReadStringLiterals(t *testing.T) {
	if got := InterpolatedTokens(Token{Kind: TokenComment, Value: `"{$a}"`}); got != nil {
		t.Errorf("InterpolatedTokens of a comment = %v, want none", got)
	}
}

func TestInterpolatedTokensArePlacedInTheFile(t *testing.T) {
	src := "<?php\n$s = \"a {$r->area()}\";\n$h = <<<EOT\n  x {$g\n  ->h}\n  EOT;\n$t = \"$u?->v\";\n"
	var got []Token
	for _, tok := range Tokenize(src) {
		got = append(got, InterpolatedTokens(tok)...)
	}
	want := []Token{
		{TokenVariable, "$r", 1, 9, 15},
		{TokenArrow, "->", 1, 11, 17},
		{TokenIdentifier, "area", 1, 13, 19},
		{TokenOpenParen, "(", 1, 17, 23},
		{TokenCloseParen, ")", 1, 18, 24},
		{TokenVariable, "$g", 3, 5, 46},
		{TokenArrow, "->", 4, 2, 51},
		{TokenIdentifier, "h", 4, 4, 53},
		{TokenVariable, "$u", 6, 6, 69},
		{TokenQuestion, "?", 6, 8, 71},
		{TokenArrow, "->", 6, 9, 72},
		{TokenIdentifier, "v", 6, 11, 74},
	}
	if !slices.Equal(got, want) {
		t.Errorf("InterpolatedTokens = %v, want %v", got, want)
	}
	for _, tok := range got {
		if src[tok.Offset:tok.Offset+len(tok.Value)] != tok.Value {
			t.Errorf("%q at offset %d reads %q", tok.Value, tok.Offset, src[tok.Offset:tok.Offset+len(tok.Value)])
		}
	}
}

func TestInterpolatedTokensHaveTheKindsOfCode(t *testing.T) {
	lit := `"$a?->b {$c->enum[$d]} {$e->f()} $g[h]"`
	code := Tokenize("<?php $a?->b $c->enum[$d] $e->f() $g")
	var want []TokenKind
	for _, tok := range code[1 : len(code)-1] {
		want = append(want, tok.Kind)
	}
	var got []TokenKind
	for _, tok := range InterpolatedTokens(Token{Kind: TokenStringLiteral, Value: lit}) {
		got = append(got, tok.Kind)
	}
	if !slices.Equal(got, want) {
		t.Errorf("kinds = %v, want %v", got, want)
	}
}
