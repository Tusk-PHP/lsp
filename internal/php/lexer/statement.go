package lexer

import "github.com/Tusk-PHP/lsp/internal/php/token"

func StatementStart(tokens []Token, first int) int {
	start, depth := first, 0
	for i := first - 1; i >= 0; i-- {
		switch kind := tokens[i].Kind; {
		case kind.IsTrivia():
			continue
		case kind == token.RightParen || kind == token.RightBracket:
			depth++
		case kind == token.LeftParen || kind == token.LeftBracket || kind == token.Attribute:
			if depth > 0 {
				depth--
			}
		case endsPreviousStatement(kind):
			if depth == 0 {
				return start
			}
		}
		start = i
	}
	return start
}

func endsPreviousStatement(kind token.Kind) bool {
	switch kind {
	case token.Semicolon, token.CloseTag, token.LeftBrace, token.RightBrace, token.OpenTag, token.OpenTagWithEcho:
		return true
	}
	return false
}
