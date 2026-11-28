package laravel

import "testing"

func TestLiteralType(t *testing.T) {
	cases := map[string]string{
		"":                    "",
		"  ":                  "",
		"42":                  "int",
		"-42":                 "int",
		"+42":                 "int",
		"1_000":               "int",
		"0x1F":                "int",
		"0b101":               "int",
		"0o17":                "int",
		"017":                 "int",
		"9223372036854775808": "float",
		"1.5":                 "float",
		".5":                  "float",
		"-.5":                 "float",
		"1e3":                 "float",
		"1_000.5":             "float",
		"'a'":                 "string",
		`"a"`:                 "string",
		`"a $b"`:              "string",
		"<<<EOT\nx\nEOT":      "string",
		"'a' . $b":            "string",
		"$b . 'a'":            "string",
		"true":                "bool",
		"FALSE":               "bool",
		`\true`:               "bool",
		"null":                "null",
		"[1, 2]":              "array",
		"array(1)":            "array",
		"-'a'":                "",
		"!1":                  "",
		"$x":                  "",
		"FOO":                 "",
		"1 +":                 "",
		"1; 2":                "",
		"foo()":               "",
		"echo 1":              "",
		"1 + 2":               "",
	}
	for expr, want := range cases {
		if got := literalType(expr); got != want {
			t.Errorf("literalType(%q) = %q, want %q", expr, got, want)
		}
	}
}
