package lexer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestMatchingClose(t *testing.T) {
	src := "<?php #[A([1, [2]])] f(g(1), {2}); [ ;"
	toks := Significant(Lex(src, token.Latest))
	for open, want := range map[int]int{1: 12, 3: 11, 4: 10, 14: 23, 16: 18, 20: 22, 2: -1, 25: -1} {
		if got := MatchingClose(toks, open); got != want {
			t.Errorf("open %d %q: close at %d, want %d", open, toks[open].Text(src), got, want)
		}
	}
}
