package sourcectx

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func isCalled(tokens []parser.Token, word protocol.Range) bool {
	last := identifierEndingAt(tokens, word.End)
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

func CallAt(uri, source string, pos protocol.Position) (callee *CursorContext, argument int, ok bool) {
	doc := NewDocument(uri, source)
	calleePos, argument, ok := calleeAt(doc.Tokens(), pos)
	if !ok {
		return nil, 0, false
	}
	callee = doc.At(calleePos)
	return callee, argument, callee != nil
}

func calleeAt(tokens []parser.Token, pos protocol.Position) (callee protocol.Position, argument int, ok bool) {
	name, argument, ok := calleeIndexAt(tokens, pos)
	if !ok {
		return protocol.Position{}, 0, false
	}
	return protocol.Position{Line: tokens[name].Line, Character: tokens[name].Column}, argument, true
}

func calleeIndexAt(tokens []parser.Token, pos protocol.Position) (name, argument int, ok bool) {
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
			if name < 0 || tokens[name].Kind != parser.TokenIdentifier {
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

// StringArgument is the quoted string the cursor is in, passed as an
// argument to a call. Class is the class name as written before `::` for a
// static call (`Env::get`), empty otherwise. Content is the range of the
// string's text between the quotes on the line it starts, Partial the text
// before the cursor.
type StringArgument struct {
	Callee   *CursorContext
	Class    string
	Argument int
	Content  protocol.Range
	Text     string
	Partial  string
}

// StringArgumentAt returns the string the cursor is in when it is itself an
// argument of a call, read from the tokens: in `env('APP_|')` it is argument
// 0 of env, in `env(['APP_|'])` or `env('A' . 'B|')` there is none.
func (d *Document) StringArgumentAt(pos protocol.Position) (StringArgument, bool) {
	tokens := d.Tokens()
	i := sort.Search(len(tokens), func(i int) bool { return !tokenEndsBeforePos(tokens[i], pos) })
	if i == len(tokens) {
		return StringArgument{}, false
	}
	tok := tokens[i]
	content, ok := quotedContent(tok, d.lines)
	if !ok || pos.Line != content.Start.Line || pos.Character < content.Start.Character || pos.Character > content.End.Character {
		return StringArgument{}, false
	}
	if prev := parser.PrevSignificant(tokens, i-1); prev < 0 || tokens[prev].Kind != parser.TokenOpenParen && tokens[prev].Kind != parser.TokenComma {
		return StringArgument{}, false
	}
	name, argument, ok := calleeIndexAt(tokens, pos)
	if !ok {
		return StringArgument{}, false
	}
	callee := d.At(protocol.Position{Line: tokens[name].Line, Character: tokens[name].Column})
	if callee == nil {
		return StringArgument{}, false
	}
	line := d.lines[content.Start.Line]
	return StringArgument{
		Callee:   callee,
		Class:    staticClassBefore(tokens, name),
		Argument: argument,
		Content:  content,
		Text:     line[content.Start.Character:content.End.Character],
		Partial:  line[content.Start.Character:pos.Character],
	}, true
}

func quotedContent(tok parser.Token, lines []string) (protocol.Range, bool) {
	v := tok.Value
	if tok.Kind != parser.TokenStringLiteral || v == "" || v[0] != '\'' && v[0] != '"' || tok.Line >= len(lines) {
		return protocol.Range{}, false
	}
	start := protocol.Position{Line: tok.Line, Character: tok.Column + 1}
	end := protocol.Position{Line: tok.Line, Character: len(lines[tok.Line])}
	if closed := len(v) >= 2 && v[len(v)-1] == v[0]; closed && tok.Column+len(v) <= end.Character {
		end.Character = tok.Column + len(v) - 1
	}
	return protocol.Range{Start: start, End: end}, true
}

func staticClassBefore(tokens []parser.Token, name int) string {
	colons := parser.PrevSignificant(tokens, name-1)
	if colons < 0 || tokens[colons].Kind != parser.TokenDoubleColon {
		return ""
	}
	end := parser.PrevSignificant(tokens, colons-1)
	start := end
	for start > 0 && isNamePart(tokens[start-1]) && isNamePart(tokens[start]) {
		start--
	}
	if end < 0 || !isNamePart(tokens[end]) {
		return ""
	}
	var b strings.Builder
	for _, tok := range tokens[start : end+1] {
		b.WriteString(tok.Value)
	}
	return b.String()
}

func isNamePart(tok parser.Token) bool {
	return tok.Kind == parser.TokenIdentifier || tok.Kind == parser.TokenBackslash
}
