package lexer

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestSignificantDropsTrivia(t *testing.T) {
	var got []token.Kind
	for _, tok := range Significant(Lex("<?php /** d */ $a = 1; // c\n# h\n/* b */ ?>x", token.Latest)) {
		got = append(got, tok.Kind)
	}
	want := []token.Kind{token.OpenTag, token.Variable, token.Equals, token.IntegerLiteral, token.Semicolon, token.CloseTag, token.InlineHTML}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
