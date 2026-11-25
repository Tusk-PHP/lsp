package parser

import (
	"slices"
	"testing"
)

func TestInterpolatedExpressions(t *testing.T) {
	for lit, want := range map[string][]string{
		`"v={$a[self::X]} and ${b}"`:              {"$a[self::X]", "b"},
		`"{$f(fn() => ['}' => 1])} {$g->h("{")}"`: {"$f(fn() => ['}' => 1])", "$g->h(\"{\")"},
		`"{$a[$m[1]]}{$b}"`:                       {"$a[$m[1]]", "$b"},
		"<<<EOT\n{$this->g(self::X)}\nEOT":        {"$this->g(self::X)"},
		"<<<\"EOT\"\n{$a}\nEOT":                   {"$a"},
		"<<<'EOT'\n{$a[self::X]}\nEOT":            nil,
		`'{$a[self::X]}'`:                         nil,
		`"self::X {\$a} \{$b} $c[0]"`:             nil,
		`"{$a[self::X]"`:                          nil,
	} {
		if got := InterpolatedExpressions(lit); !slices.Equal(got, want) {
			t.Errorf("InterpolatedExpressions(%s) = %q, want %q", lit, got, want)
		}
	}
}
