package lexer

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestLiteralEnd(t *testing.T) {
	for src, want := range map[string]string{
		`<?php "a {$b["k"]} $c" . 1;`:         `"a {$b["k"]} $c"`,
		`<?php "a {$f("x$y")} ${z}" . 1;`:     `"a {$f("x$y")} ${z}"`,
		"<?php <<<EOT\n{$f(`ls $d`)}\nEOT;\n": "<<<EOT\n{$f(`ls $d`)}\nEOT",
		"<?php `ls $dir` . 1;":                "`ls $dir`",
		`<?php "open {$k more`:                `"open {$k more`,
	} {
		toks := Lex(src, token.Latest)
		open := 0
		for !OpensLiteral(toks[open].Kind) {
			open++
		}
		end := LiteralEnd(toks, open)
		if got := src[toks[open].Start:toks[end].End]; got != want {
			t.Errorf("%q: got %q, want %q", src, got, want)
		}
	}
}

func TestWholeLiterals(t *testing.T) {
	src := "<?php $a = \"x {$b} y\" . <<<EOT\n$c\nEOT;"
	var got []string
	for _, tok := range WholeLiterals(Significant(Lex(src, token.Latest))) {
		got = append(got, tok.Text(src))
	}
	want := []string{"<?php ", "$a", "=", "\"x {$b} y\"", ".", "<<<EOT\n$c\nEOT", ";"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
