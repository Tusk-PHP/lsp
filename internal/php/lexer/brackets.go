package lexer

import "github.com/Tusk-PHP/lsp/internal/php/token"

var closingKinds = map[token.Kind]token.Kind{
	token.LeftParen:   token.RightParen,
	token.LeftBracket: token.RightBracket,
	token.Attribute:   token.RightBracket,
	token.LeftBrace:   token.RightBrace,
}

func MatchingClose(tokens []Token, open int) int {
	closer, ok := closingKinds[tokens[open].Kind]
	if !ok {
		return -1
	}
	depth := 0
	for i := open; i < len(tokens); i++ {
		kind := tokens[i].Kind
		switch {
		case closingKinds[kind] == closer:
			depth++
		case kind == closer:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
