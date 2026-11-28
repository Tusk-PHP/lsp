package lexer

import "github.com/Tusk-PHP/lsp/internal/php/token"

var literalClosers = map[token.Kind]token.Kind{
	token.DoubleQuote:  token.DoubleQuote,
	token.StartHeredoc: token.EndHeredoc,
	token.Backtick:     token.Backtick,
}

func OpensLiteral(kind token.Kind) bool {
	_, ok := literalClosers[kind]
	return ok
}

func LiteralEnd(tokens []Token, open int) int {
	closer := literalClosers[tokens[open].Kind]
	depth := 0
	for i := open + 1; i < len(tokens); i++ {
		kind := tokens[i].Kind
		switch {
		case kind == token.CurlyOpen || kind == token.DollarOpenCurlyBraces || kind == token.LeftBrace:
			depth++
		case kind == token.RightBrace:
			depth--
		case depth > 0 && OpensLiteral(kind):
			i = LiteralEnd(tokens, i)
		case depth == 0 && kind == closer:
			return i
		}
	}
	return len(tokens) - 1
}

func WholeLiterals(tokens []Token) []Token {
	out := make([]Token, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if OpensLiteral(tok.Kind) {
			i = LiteralEnd(tokens, i)
			tok.End = tokens[i].End
		}
		out = append(out, tok)
	}
	return out
}
