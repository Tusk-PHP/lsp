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
	all := lexer.Significant(lexer.Lex(src, token.Latest))
	lines := ast.NewLines(src)
	toks := make([]phpToken, 0, len(all))
	for i := 0; i < len(all); i++ {
		t := all[i]
		kind, end := t.Kind, t.End
		switch kind {
		case token.InlineHTML:
			continue
		case token.CloseTag:
			kind = token.Semicolon
		case token.DoubleQuote, token.StartHeredoc, token.Backtick:
			i = stringEnd(all, i)
			end = all[i].End
		}
		line, col := lines.Position(t.Start)
		toks = append(toks, phpToken{Kind: kind, Value: src[t.Start:end], Line: line, Column: col, Offset: int(t.Start)})
	}
	return toks
}

func stringEnd(all []lexer.Token, open int) int {
	closer := all[open].Kind
	if closer == token.StartHeredoc {
		closer = token.EndHeredoc
	}
	depth := 0
	for i := open + 1; i < len(all); i++ {
		switch kind := all[i].Kind; {
		case depth == 0 && kind == closer:
			return i
		case kind == token.CurlyOpen || kind == token.DollarOpenCurlyBraces || kind == token.LeftBrace:
			depth++
		case kind == token.RightBrace:
			depth--
		case depth > 0 && (kind == token.DoubleQuote || kind == token.StartHeredoc || kind == token.Backtick):
			i = stringEnd(all, i)
		}
	}
	return len(all) - 1
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
