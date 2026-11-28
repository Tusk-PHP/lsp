package scope

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

const endOfFile = token.Invalid

type tokens struct {
	src     string
	list    []lexer.Token
	ranges  []textpos.Range
	lines   ast.Lines
	strings []stringVariable
}

type stringVariable struct {
	literal int
	form    stringForm
	name    string
	at      textpos.Range
}

type stringForm uint8

const (
	stringVariableName stringForm = iota
	stringBracedName
	stringComputedName
)

func lexTokens(src string) *tokens {
	all := lexer.Lex(src, token.Latest)
	if len(all) == 1 && all[0].Kind == token.InlineHTML {
		all = lexer.LexCode(src, token.Latest)
	}
	t := &tokens{src: src, list: make([]lexer.Token, 0, len(all)/2+1), lines: ast.NewLines(src)}
	for i := 0; i < len(all); i++ {
		tok := all[i]
		if tok.Kind.IsTrivia() || tok.Kind == token.InlineHTML {
			continue
		}
		if lexer.OpensLiteral(tok.Kind) {
			end := lexer.LiteralEnd(all, i)
			t.addStringVariables(all[i+1:end+1], len(t.list))
			tok.End = all[end].End
			i = end
		}
		t.list = append(t.list, tok)
	}
	n := uint32(len(src))
	t.list = append(t.list, lexer.Token{Kind: endOfFile, Start: n, End: n})
	t.ranges = make([]textpos.Range, len(t.list))
	for i, tok := range t.list {
		t.ranges[i] = t.rangeOf(tok)
	}
	return t
}

func (t *tokens) addStringVariables(inner []lexer.Token, literal int) {
	for j := 0; j < len(inner); j++ {
		switch inner[j].Kind {
		case token.Variable:
			t.strings = append(t.strings, stringVariable{literal: literal, form: stringVariableName, name: inner[j].Text(t.src), at: t.rangeOf(inner[j])})
		case token.DollarOpenCurlyBraces:
			if j+1 == len(inner) || inner[j+1].Kind != token.StringVarname {
				t.strings = append(t.strings, stringVariable{literal: literal, form: stringComputedName})
				continue
			}
			j++
			t.strings = append(t.strings, stringVariable{literal: literal, form: stringBracedName, name: "$" + inner[j].Text(t.src), at: t.rangeOf(inner[j])})
		}
	}
}

func (t *tokens) position(offset uint32) textpos.Position {
	line, col := t.lines.Position(offset)
	return textpos.Position{Line: line, Character: col}
}

func (t *tokens) rangeOf(tok lexer.Token) textpos.Range {
	return textpos.Range{Start: t.position(tok.Start), End: t.position(tok.End)}
}

func (t *tokens) kind(i int) token.Kind { return t.list[i].Kind }

func (t *tokens) text(i int) string { return t.list[i].Text(t.src) }

func (t *tokens) rng(i int) textpos.Range { return t.ranges[i] }

func (t *tokens) fileEnd() textpos.Position { return t.ranges[len(t.ranges)-1].End }

func (t *tokens) next(from int) int {
	from = max(from, 0)
	if from < len(t.list) {
		return from
	}
	return -1
}

func (t *tokens) prev(from int) int {
	from = min(from, len(t.list)-1)
	if from < 0 {
		return -1
	}
	return from
}

func (t *tokens) lastSegment(i int) (string, textpos.Range) {
	text := t.text(i)
	cut := strings.LastIndexByte(text, '\\') + 1
	return text[cut:], textpos.Range{Start: t.position(t.list[i].Start + uint32(cut)), End: t.rng(i).End}
}

func (t *tokens) matchingClose(open int) int {
	return lexer.MatchingClose(t.list, open)
}

func (t *tokens) matchingOpen(close int) int {
	closer := t.kind(close)
	depth := 0
	for i := close; i >= 0; i-- {
		switch k := t.kind(i); {
		case k == closer:
			depth++
		case closerOf(k) == closer:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func (t *tokens) arrowExpressionEnd(start int) int {
	depth := 0
	for i := start; i < len(t.list); i++ {
		switch k := t.kind(i); {
		case opensBracket(k):
			depth++
		case closesBracket(k):
			if depth == 0 {
				return t.prev(i - 1)
			}
			depth--
		case k == token.Comma || isStatementEnd(k):
			if depth == 0 {
				return t.prev(i - 1)
			}
		}
	}
	return t.prev(len(t.list) - 1)
}

func closerOf(open token.Kind) token.Kind {
	switch open {
	case token.LeftParen:
		return token.RightParen
	case token.LeftBracket, token.Attribute:
		return token.RightBracket
	case token.LeftBrace:
		return token.RightBrace
	}
	return token.Invalid
}

func opensBracket(k token.Kind) bool { return closerOf(k) != token.Invalid }

func closesBracket(k token.Kind) bool {
	return k == token.RightParen || k == token.RightBracket || k == token.RightBrace
}

func opensSquare(k token.Kind) bool { return k == token.LeftBracket || k == token.Attribute }

func isStatementEnd(k token.Kind) bool { return k == token.Semicolon || k == token.CloseTag }

func isName(k token.Kind) bool {
	switch k {
	case token.Identifier, token.NameQualified, token.NameFullyQualified, token.NameRelative:
		return true
	}
	return false
}

func isMemberAccess(k token.Kind) bool {
	return k == token.ObjectOperator || k == token.NullsafeObjectOperator
}

func isAmpersand(k token.Kind) bool {
	switch k {
	case token.Ampersand, token.AmpersandFollowedByVarOrVararg, token.AmpersandNotFollowedByVarOrVararg:
		return true
	}
	return false
}
