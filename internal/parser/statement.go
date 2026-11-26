package parser

// StatementStart returns the index of the first token of the statement that
// contains tokens[first]. It scans backwards to the nearest token that ends
// the previous statement and is not nested inside parentheses or brackets:
// ';' (a '?>' close tag is lexed as one), '{', '}' or an open tag ('<?php' or
// '<?='). Comments between that token and the statement are skipped. Without
// such a token the statement starts at the first token of the slice.
func StatementStart(tokens []Token, first int) int {
	start, depth := first, 0
	for i := first - 1; i >= 0; i-- {
		switch tokens[i].Kind {
		case TokenComment, TokenDocComment:
			continue
		case TokenCloseParen, TokenCloseBracket:
			depth++
		case TokenOpenParen, TokenOpenBracket:
			if depth > 0 {
				depth--
			}
		case TokenSemicolon, TokenOpenBrace, TokenCloseBrace, TokenPHP:
			if depth == 0 {
				return start
			}
		}
		start = i
	}
	return start
}
