package sourcectx

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func isCalled(tokens []parser.Token, word protocol.Range) bool {
	tokens, last := identifierEndingAt(tokens, word.End)
	if last < 0 {
		return false
	}
	if next := parser.NextSignificant(tokens, last+1); next < 0 || tokens[next].Kind != parser.TokenOpenParen {
		return false
	}
	first := last
	for first > 0 && (tokens[first-1].Kind == parser.TokenIdentifier || tokens[first-1].Kind == parser.TokenBackslash) {
		first--
	}
	if prev := parser.PrevSignificant(tokens, first-1); prev >= 0 {
		if tokens[prev].Kind == parser.TokenAmpersand {
			if beforeRef := parser.PrevSignificant(tokens, prev-1); beforeRef >= 0 && tokens[beforeRef].Kind == parser.TokenFunction {
				return false
			}
		}
		switch tokens[prev].Kind {
		case parser.TokenNew, parser.TokenFunction:
			return false
		}
	}
	return !inAttribute(tokens, first)
}

func identifierEndingAt(tokens []parser.Token, end protocol.Position) ([]parser.Token, int) {
	i := sort.Search(len(tokens), func(i int) bool {
		t := tokens[i]
		return t.Line > end.Line || t.Line == end.Line && t.Column+len(t.Value) >= end.Character
	})
	for _, k := range []int{i - 1, i} {
		if k >= 0 && k < len(tokens) && tokens[k].Kind == parser.TokenStringLiteral && spans(tokens[k], end) {
			return identifierEndingAt(parser.InterpolatedTokens(tokens[k]), end)
		}
	}
	if i == len(tokens) {
		return tokens, -1
	}
	t := tokens[i]
	if t.Kind != parser.TokenIdentifier || t.Line != end.Line || t.Column+len(t.Value) != end.Character {
		return tokens, -1
	}
	return tokens, i
}

func spans(tok parser.Token, pos protocol.Position) bool {
	if pos.Line < tok.Line || pos.Line == tok.Line && pos.Character <= tok.Column {
		return false
	}
	lastLine, lastColumn := tok.Line, tok.Column+len(tok.Value)
	if nl := strings.LastIndexByte(tok.Value, '\n'); nl >= 0 {
		lastLine, lastColumn = tok.Line+strings.Count(tok.Value, "\n"), len(tok.Value)-nl-1
	}
	return pos.Line < lastLine || pos.Line == lastLine && pos.Character < lastColumn
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

func CallAt(uri, source string, pos protocol.Position) (callee *CursorContext, argument int, ok bool) {
	return NewDocument(uri, source).CallAt(pos)
}

// CallAt is the package's CallAt on an already parsed document.
func (doc *Document) CallAt(pos protocol.Position) (callee *CursorContext, argument int, ok bool) {
	calleePos, argument, ok := calleeAt(doc.Tokens(), pos)
	if !ok {
		return nil, 0, false
	}
	callee = doc.At(calleePos)
	return callee, argument, callee != nil
}

// CalledVariableAt finds the call pos is in the arguments of when what is
// called is a variable, `$fn(…)`: the variable's name and position, and
// which argument pos is in. A dynamic member call (`$obj->$name(…)`,
// `Foo::$name(…)`) and `new $class(…)` don't count.
func (doc *Document) CalledVariableAt(pos protocol.Position) (name string, at protocol.Position, argument int, ok bool) {
	tokens := doc.Tokens()
	callee, argument, ok := calleeTokenAt(tokens, pos)
	if !ok || tokens[callee].Kind != parser.TokenVariable {
		return "", protocol.Position{}, 0, false
	}
	if prev := parser.PrevSignificant(tokens, callee-1); prev >= 0 {
		switch tokens[prev].Kind {
		case parser.TokenArrow, parser.TokenDoubleColon, parser.TokenNew:
			return "", protocol.Position{}, 0, false
		}
	}
	tok := tokens[callee]
	return tok.Value, protocol.Position{Line: tok.Line, Character: tok.Column}, argument, true
}

func calleeAt(tokens []parser.Token, pos protocol.Position) (callee protocol.Position, argument int, ok bool) {
	i, argument, ok := calleeTokenAt(tokens, pos)
	if !ok || tokens[i].Kind != parser.TokenIdentifier {
		return protocol.Position{}, 0, false
	}
	return protocol.Position{Line: tokens[i].Line, Character: tokens[i].Column}, argument, true
}

// calleeTokenAt returns the index of the token right before the `(` of the
// call whose arguments pos is in, and which argument that is.
func calleeTokenAt(tokens []parser.Token, pos protocol.Position) (callee int, argument int, ok bool) {
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
				return 0, 0, false
			}
			depth--
		case parser.TokenOpenParen:
			if depth > 0 {
				depth--
				continue
			}
			name := parser.PrevSignificant(tokens, k-1)
			if name < 0 {
				return 0, 0, false
			}
			return name, argument, true
		case parser.TokenComma:
			if depth == 0 {
				argument++
			}
		case parser.TokenSemicolon:
			if depth == 0 {
				return 0, 0, false
			}
		}
	}
	return 0, 0, false
}

func tokenEndsBeforePos(tok parser.Token, pos protocol.Position) bool {
	return tok.Line < pos.Line || tok.Line == pos.Line && tok.Column+len(tok.Value) <= pos.Character
}
