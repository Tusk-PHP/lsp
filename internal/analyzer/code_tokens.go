package analyzer

import (
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

type codeTokens struct {
	src   string
	lines ast.Lines
	all   []lexer.Token
}

func newCodeTokens(src string, lexed []lexer.Token) codeTokens {
	return codeTokens{src: src, lines: ast.NewLines(src), all: lexer.WholeLiterals(lexer.Significant(lexed))}
}

func (t codeTokens) kind(i int) token.Kind {
	if i < 0 || i >= len(t.all) {
		return token.Invalid
	}
	return t.all[i].Kind
}

func (t codeTokens) text(i int) string {
	return t.all[i].Text(t.src)
}

func (t codeTokens) position(offset uint32) protocol.Position {
	line, col := t.lines.Position(offset)
	return protocol.Position{Line: line, Character: col}
}

func (t codeTokens) offset(pos protocol.Position) (int, bool) {
	if pos.Line < 0 || pos.Line >= len(t.lines) || pos.Character < 0 {
		return 0, false
	}
	end := len(t.src)
	if pos.Line+1 < len(t.lines) {
		end = int(t.lines[pos.Line+1]) - 1
	}
	offset := int(t.lines[pos.Line]) + pos.Character
	return offset, offset <= end
}

func (t codeTokens) line(n int) string {
	end := len(t.src)
	if n+1 < len(t.lines) {
		end = int(t.lines[n+1]) - 1
	}
	return t.src[t.lines[n]:end]
}
