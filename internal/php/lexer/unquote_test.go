package lexer

import "testing"

func TestUnquote(t *testing.T) {
	cases := []struct {
		lit  string
		want string
		ok   bool
	}{
		{`'routes/web.php'`, `routes/web.php`, true},
		{`'a\'b\\c\d'`, `a'b\c\d`, true},
		{`"a\"b\\c\nd\$e"`, "a\"b\\c\nd$e", true},
		{`"path/$name.php"`, "", false},
		{`"\x41"`, "", false},
		{`"\u{41}"`, "", false},
		{`"\101"`, "", false},
		{`"App\Repository\UserRepository"`, `App\Repository\UserRepository`, true},
		{`"trailing\"`, `trailing\`, true},
		{"<<<EOT\nx\nEOT", "", false},
		{`'mismatched"`, "", false},
		{`'`, "", false},
	}
	for _, c := range cases {
		got, ok := Unquote(c.lit)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got %q, %v; want %q, %v", c.lit, got, ok, c.want, c.ok)
		}
	}
}
