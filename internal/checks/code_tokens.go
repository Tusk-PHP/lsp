package checks

import (
	"cmp"
	"slices"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func isObjectOperator(kind token.Kind) bool {
	return kind == token.ObjectOperator || kind == token.NullsafeObjectOperator
}

func isMemberAccess(kind token.Kind) bool {
	return isObjectOperator(kind) || kind == token.DoubleColon
}

func lineSpan(lines ast.Lines, first, last, size int) (from, to uint32) {
	from, to = lines[first], uint32(size)
	if last+1 < len(lines) {
		to = lines[last+1]
	}
	return from, to
}

func tokensWithin(tokens []lexer.Token, from, to uint32) []lexer.Token {
	byStart := func(t lexer.Token, offset uint32) int { return cmp.Compare(t.Start, offset) }
	first, _ := slices.BinarySearchFunc(tokens, from, byStart)
	last, _ := slices.BinarySearchFunc(tokens, to, byStart)
	return tokens[first:last]
}
