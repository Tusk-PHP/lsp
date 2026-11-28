package analyzer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestCodeTokensOffset(t *testing.T) {
	src := "<?php\n$é = 1;\n"
	toks := newCodeTokens(src, lexer.Lex(src, token.Latest))
	for _, tc := range []struct {
		pos  protocol.Position
		want int
		ok   bool
	}{
		{protocol.Position{Line: 1, Character: 3}, 9, true},
		{protocol.Position{Line: 1, Character: 8}, 14, true},
		{protocol.Position{Line: 1, Character: 9}, 0, false},
		{protocol.Position{Line: 2}, 15, true},
		{protocol.Position{Line: 3}, 0, false},
		{protocol.Position{Line: -1}, 0, false},
		{protocol.Position{Line: 1, Character: -1}, 0, false},
	} {
		got, ok := toks.offset(tc.pos)
		if ok != tc.ok || ok && got != tc.want {
			t.Errorf("offset(%v) = %d, %v; want %d, %v", tc.pos, got, ok, tc.want, tc.ok)
		}
	}
	if got := toks.line(1); got != "$é = 1;" {
		t.Errorf("line(1) = %q", got)
	}
}
