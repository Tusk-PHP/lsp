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
