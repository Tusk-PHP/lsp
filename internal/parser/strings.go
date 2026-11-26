package parser

import "github.com/Tusk-PHP/lsp/internal/php/lexer"

func ConstantString(tok Token) (string, bool) {
	if tok.Kind != TokenStringLiteral {
		return "", false
	}
	return lexer.Unquote(tok.Value)
}
