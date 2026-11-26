package ast

import (
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func (f *File) ReturnedArray() *Array {
	for _, stmt := range f.Stmts {
		if ret, ok := stmt.(*Return); ok {
			array, _ := ret.Value.(*Array)
			return array
		}
	}
	return nil
}

func StringValue(e Expr, src string) (string, bool) {
	lit, ok := e.(*Literal)
	if !ok || lit.Kind != token.ConstantString {
		return "", false
	}
	return lexer.Unquote(lit.Text(src))
}
