package parser

import "testing"

func TestConstantString(t *testing.T) {
	cases := []struct {
		tok  Token
		want string
		ok   bool
	}{
		{Token{Kind: TokenStringLiteral, Value: `'routes/web.php'`}, `routes/web.php`, true},
		{Token{Kind: TokenStringLiteral, Value: `'a\'b\\c\d'`}, `a'b\c\d`, true},
		{Token{Kind: TokenStringLiteral, Value: `"a\"b\\c\nd\$e"`}, "a\"b\\c\nd$e", true},
		{Token{Kind: TokenStringLiteral, Value: `"path/$name.php"`}, "", false},
		{Token{Kind: TokenStringLiteral, Value: `"\x41"`}, "", false},
		{Token{Kind: TokenStringLiteral, Value: `"\101"`}, "", false},
		{Token{Kind: TokenStringLiteral, Value: `"\u{41}"`}, "", false},
		{Token{Kind: TokenStringLiteral, Value: `"App\Config\VERSION"`}, `App\Config\VERSION`, true},
		{Token{Kind: TokenStringLiteral, Value: `"trailing\"`}, `trailing\`, true},
		{Token{Kind: TokenStringLiteral, Value: "<<<EOT\nx\nEOT"}, "", false},
		{Token{Kind: TokenStringLiteral, Value: `'mismatched"`}, "", false},
		{Token{Kind: TokenStringLiteral, Value: `'`}, "", false},
		{Token{Kind: TokenIdentifier, Value: `'x'`}, "", false},
	}
	for _, c := range cases {
		got, ok := ConstantString(c.tok)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got %q, %v; want %q, %v", c.tok.Value, got, ok, c.want, c.ok)
		}
	}
}
