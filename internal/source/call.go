package sourcectx

import (
	"sort"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func isFunctionCall(tokens []parser.Token, word protocol.Range) bool {
	last := identifierEndingAt(tokens, word.End)
	if last < 0 {
		return false
	}
	if next := nextSignificantToken(tokens, last+1); next < 0 || tokens[next].Kind != parser.TokenOpenParen {
		return false
	}
	first := last
	for first > 0 && (tokens[first-1].Kind == parser.TokenIdentifier || tokens[first-1].Kind == parser.TokenBackslash) {
		first--
	}
	if prev := prevSignificantToken(tokens, first-1); prev >= 0 {
		switch tokens[prev].Kind {
		case parser.TokenNew, parser.TokenFunction, parser.TokenArrow, parser.TokenDoubleColon:
			return false
		}
	}
	return !inAttribute(tokens, first)
}

func identifierEndingAt(tokens []parser.Token, end protocol.Position) int {
	i := sort.Search(len(tokens), func(i int) bool {
		t := tokens[i]
		return t.Line > end.Line || t.Line == end.Line && t.Column+len(t.Value) >= end.Character
	})
	if i == len(tokens) {
		return -1
	}
	t := tokens[i]
	if t.Kind != parser.TokenIdentifier || t.Line != end.Line || t.Column+len(t.Value) != end.Character {
		return -1
	}
	return i
}

func inAttribute(tokens []parser.Token, i int) bool {
	depth := 0
	for k := i - 1; k >= 0; k-- {
		switch tokens[k].Kind {
		case parser.TokenCloseBracket, parser.TokenCloseParen:
			depth++
		case parser.TokenOpenParen:
			if depth > 0 {
				depth--
			}
		case parser.TokenOpenBracket:
			if depth > 0 {
				depth--
				continue
			}
			if k > 0 && tokens[k-1].Kind == parser.TokenHash {
				return true
			}
		case parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace:
			return false
		}
	}
	return false
}

func CallAt(source string, pos protocol.Position) (callee protocol.Position, argument int, ok bool) {
	tokens := parser.Tokenize(source)
	cursor := sort.Search(len(tokens), func(i int) bool { return !tokenEndsBeforePos(tokens[i], pos) })
	depth := 0
	for k := cursor - 1; k >= 0; k-- {
		switch tokens[k].Kind {
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth++
		case parser.TokenOpenBracket:
			if depth > 0 {
				depth--
				continue
			}
			argument = 0
		case parser.TokenOpenBrace:
			if depth == 0 {
				return protocol.Position{}, 0, false
			}
			depth--
		case parser.TokenOpenParen:
			if depth > 0 {
				depth--
				continue
			}
			name := prevSignificantToken(tokens, k-1)
			if name < 0 || tokens[name].Kind != parser.TokenIdentifier {
				return protocol.Position{}, 0, false
			}
			return protocol.Position{Line: tokens[name].Line, Character: tokens[name].Column}, argument, true
		case parser.TokenComma:
			if depth == 0 {
				argument++
			}
		case parser.TokenSemicolon:
			if depth == 0 {
				return protocol.Position{}, 0, false
			}
		}
	}
	return protocol.Position{}, 0, false
}

func tokenEndsBeforePos(tok parser.Token, pos protocol.Position) bool {
	return tok.Line < pos.Line || tok.Line == pos.Line && tok.Column+len(tok.Value) <= pos.Character
}
