package lexer

func Significant(tokens []Token) []Token {
	out := make([]Token, 0, len(tokens))
	for _, tok := range tokens {
		if !tok.Kind.IsTrivia() {
			out = append(out, tok)
		}
	}
	return out
}
