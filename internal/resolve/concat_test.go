package resolve

import "testing"

func TestIsConcatenation(t *testing.T) {
	for expr, want := range map[string]bool{
		`$front . 'date/'`:                  true,
		`$dir . '/' . $term`:                true,
		`'a'.'b'`:                           true,
		`$a1.'x'`:                           true,
		`$x->name . "-" . $x->id`:           true,
		`Foo::PREFIX . $key`:                true,
		`$n + 1 . 'px'`:                     true,
		`!$a . 'x'`:                         true,
		`strtolower($a . $b)`:               false, // inside parentheses: the call decides
		`[$a . $b]`:                         false,
		`'a.b'`:                             false, // in a string
		`"a{$b}.c"`:                         false,
		`'it\'s.' . 'x'`:                    true,
		`1.5`:                               false, // a number
		`.5 + $x`:                           false,
		`1. + 2`:                            false,
		`foo(...$args)`:                     false,
		`[...$a, ...$b]`:                    false,
		`$s .= 'x'`:                         false,
		`$a . $b == $c`:                     false, // a comparison
		`$a . 'x' ?? 'y'`:                   false,
		`$c ? 'a' . 'b' : 'c'`:              false,
		`$a . 'x' and $b`:                   false,
		`$a . 'x' instanceof Foo`:           false,
		`$x = $a . $b`:                      false,
		`$a->b() // a.b`:                    false, // a comment isn't read
		`$a # x.y`:                          false,
		`fn () => $a . $b`:                  false,
		`function () { return 'a' . 'b'; }`: false,
		`<<<EOT`:                            false,
		`$a?->b . 'x'`:                      true,
		`$a << 2 . 'x'`:                     true,
		`$x`:                                false,
		`'unterminated . x`:                 false,
	} {
		if got := isConcatenation(expr); got != want {
			t.Errorf("isConcatenation(%q) = %v, want %v", expr, got, want)
		}
	}
}

func TestConcatenationIsAString(t *testing.T) {
	r := NewResolver(nil)
	if got := r.ExpressionType(`$dir . '/' . $name`, ExprContext{}).FullString(); got != "string" {
		t.Errorf("ExpressionType = %q, want string", got)
	}
}
