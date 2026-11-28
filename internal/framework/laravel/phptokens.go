package laravel

import (
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type phpToken struct {
	Kind   token.Kind
	Value  string
	Line   int
	Column int
	Offset int
}

func phpTokens(src string) []phpToken {
	all := lexer.WholeLiterals(lexer.Significant(lexer.Lex(src, token.Latest)))
	lines := ast.NewLines(src)
	toks := make([]phpToken, 0, len(all))
	for _, t := range all {
		kind := t.Kind
		switch kind {
		case token.InlineHTML:
			continue
		case token.CloseTag:
			kind = token.Semicolon
		}
		line, col := lines.Position(t.Start)
		toks = append(toks, phpToken{Kind: kind, Value: t.Text(src), Line: line, Column: col, Offset: int(t.Start)})
	}
	return toks
}

func isArrow(kind token.Kind) bool {
	return kind == token.ObjectOperator || kind == token.NullsafeObjectOperator
}

func isName(kind token.Kind) bool {
	return kind == token.Identifier || kind == token.NameQualified || kind == token.NameFullyQualified
}

var closers = map[token.Kind]token.Kind{token.LeftParen: token.RightParen, token.LeftBracket: token.RightBracket, token.LeftBrace: token.RightBrace}

func matchingClose(toks []phpToken, open int) int {
	if open >= len(toks) {
		return -1
	}
	openKind := toks[open].Kind
	closeKind, ok := closers[openKind]
	if !ok {
		return -1
	}
	depth := 0
	for k := open; k < len(toks); k++ {
		switch kind := toks[k].Kind; {
		case kind == openKind || openKind == token.LeftBracket && kind == token.Attribute:
			depth++
		case kind == closeKind:
			depth--
			if depth == 0 {
				return k
			}
		}
	}
	return -1
}
