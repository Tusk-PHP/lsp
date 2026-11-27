package hover

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

const maxValuePreview = 160

type previewCut struct {
	end  int
	open []token.Kind
}

func valuePreview(value string) string {
	if len(value) <= maxValuePreview {
		return value
	}
	prefix := value[:maxValuePreview+1]
	var open []token.Kind
	var atToken, atComma previewCut
	inString := false
	for _, tok := range lexer.LexCode(prefix, token.Latest) {
		if int(tok.End) >= len(prefix) {
			break
		}
		switch tok.Kind {
		case token.DoubleQuote, token.Backtick:
			inString = !inString
		case token.StartHeredoc:
			inString = true
		case token.EndHeredoc:
			inString = false
		}
		if inString {
			continue
		}
		switch tok.Kind {
		case token.LeftBracket, token.LeftParen, token.LeftBrace:
			open = append(open, tok.Kind)
		case token.RightBracket, token.RightParen, token.RightBrace:
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
		cut := previewCut{end: int(tok.End), open: append([]token.Kind(nil), open...)}
		atToken = cut
		if tok.Kind == token.Comma {
			atComma = cut
		}
	}
	cut := atToken
	if atComma.end > 0 {
		cut = atComma
	}
	var b strings.Builder
	b.WriteString(prefix[:cut.end])
	b.WriteString("…")
	for i := len(cut.open) - 1; i >= 0; i-- {
		b.WriteString(closer(cut.open[i]))
	}
	return b.String()
}

func closer(open token.Kind) string {
	switch open {
	case token.LeftBracket:
		return "]"
	case token.LeftParen:
		return ")"
	}
	return "}"
}
