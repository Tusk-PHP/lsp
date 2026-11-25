package parser

func MatchingClose(tokens []Token, open int) int {
	if open < 0 || open >= len(tokens) {
		return -1
	}
	openKind := tokens[open].Kind
	closeKind, ok := closingKind(openKind)
	if !ok {
		return -1
	}
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case openKind:
			depth++
		case closeKind:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func MatchingOpen(tokens []Token, close int) int {
	if close < 0 || close >= len(tokens) {
		return -1
	}
	closeKind := tokens[close].Kind
	openKind, ok := openingKind(closeKind)
	if !ok {
		return -1
	}
	depth := 0
	for i := close; i >= 0; i-- {
		switch tokens[i].Kind {
		case closeKind:
			depth++
		case openKind:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func closingKind(open TokenKind) (TokenKind, bool) {
	switch open {
	case TokenOpenParen:
		return TokenCloseParen, true
	case TokenOpenBracket:
		return TokenCloseBracket, true
	case TokenOpenBrace:
		return TokenCloseBrace, true
	}
	return 0, false
}

func openingKind(close TokenKind) (TokenKind, bool) {
	switch close {
	case TokenCloseParen:
		return TokenOpenParen, true
	case TokenCloseBracket:
		return TokenOpenBracket, true
	case TokenCloseBrace:
		return TokenOpenBrace, true
	}
	return 0, false
}
