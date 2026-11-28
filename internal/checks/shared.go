package checks

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

// Shared holds what several rules run over the same file would each work
// out again: the file's tokens and structure. Make one per run over one
// version of a file and give it to each rule; a rule without one parses the
// file itself.
type Shared struct {
	source string
	result *parser.ParseResult
	parsed bool

	lexedSource string
	lexed       []lexer.Token
}

// parse returns the parse of source, done once per Shared and source. The
// result is shared between rules: read it, don't change it.
func (s *Shared) parse(source string) *parser.ParseResult {
	if s == nil {
		return parser.New().Parse(source)
	}
	if !s.parsed || s.source != source {
		s.source, s.result, s.parsed = source, parser.New().Parse(source), true
	}
	return s.result
}

func (s *Shared) lex(source string) []lexer.Token {
	if s == nil {
		return lexer.Lex(source, token.Latest)
	}
	if s.lexed == nil || s.lexedSource != source {
		s.lexedSource, s.lexed = source, lexer.Lex(source, token.Latest)
	}
	return s.lexed
}
