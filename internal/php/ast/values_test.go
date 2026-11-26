package ast_test

import (
	"testing"

	phpast "github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestReturnedArray(t *testing.T) {
	cases := []struct {
		src   string
		items int
	}{
		{"<?php\n$a = 1;\nreturn ['a' => 1, 'b' => 2];\nreturn [];\n", 2},
		{"<?php\nreturn array('a');\n", 1},
		{"<?php\nreturn 1;\n", -1},
		{"<?php\nreturn;\n", -1},
		{"<?php\nfunction f() { return [1]; }\n", -1},
	}
	for _, c := range cases {
		array := parser.Parse(c.src, token.Latest).ReturnedArray()
		got := -1
		if array != nil {
			got = len(array.Items)
		}
		if got != c.items {
			t.Errorf("%q: %d items, want %d", c.src, got, c.items)
		}
	}
}

func TestStringValue(t *testing.T) {
	cases := []struct {
		expr string
		want string
		ok   bool
	}{
		{`'a\'b'`, `a'b`, true},
		{`"a\tb"`, "a\tb", true},
		{`"a $b"`, "", false},
		{`1`, "", false},
		{`FOO`, "", false},
	}
	for _, c := range cases {
		src := "<?php\nreturn [" + c.expr + "];\n"
		array := parser.Parse(src, token.Latest).ReturnedArray()
		got, ok := phpast.StringValue(array.Items[0].Value, src)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got %q, %v; want %q, %v", c.expr, got, ok, c.want, c.ok)
		}
	}
}
