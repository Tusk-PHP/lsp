package parser

func (k TokenKind) IsComment() bool {
	return k == TokenComment || k == TokenDocComment
}

func NextSignificant(tokens []Token, from int) int {
	for i := max(from, 0); i < len(tokens); i++ {
		if !tokens[i].Kind.IsComment() {
			return i
		}
	}
	return -1
}

func PrevSignificant(tokens []Token, from int) int {
	for i := min(from, len(tokens)-1); i >= 0; i-- {
		if !tokens[i].Kind.IsComment() {
			return i
		}
	}
	return -1
}

func SignificantTokens(tokens []Token) []Token {
	out := make([]Token, 0, len(tokens))
	for _, tok := range tokens {
		if !tok.Kind.IsComment() {
			out = append(out, tok)
		}
	}
	return out
}

func IsNamespaceDeclaration(tokens []Token, at int) bool {
	if prev := PrevSignificant(tokens, at-1); prev >= 0 {
		if k := tokens[prev].Kind; k == TokenArrow || k == TokenDoubleColon {
			return false
		}
	}
	next := NextSignificant(tokens, at+1)
	return next >= 0 && (tokens[next].Kind == TokenIdentifier || tokens[next].Kind == TokenOpenBrace)
}
