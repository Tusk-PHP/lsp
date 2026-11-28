package checks

import "github.com/Tusk-PHP/lsp/internal/parser"

// Shared holds what several rules run over the same file would each work
// out again: the file's tokens and structure. Make one per run over one
// version of a file and give it to each rule; a rule without one parses the
// file itself.
type Shared struct {
	source      string
	result      *parser.ParseResult
	parsed      bool
	significant []parser.Token
}

// parse returns the parse of source, done once per Shared and source. The
// result is shared between rules: read it, don't change it.
func (s *Shared) parse(source string) *parser.ParseResult {
	if s == nil {
		return parser.New().Parse(source)
	}
	if !s.parsed || s.source != source {
		s.source, s.result, s.parsed = source, parser.New().Parse(source), true
		s.significant = nil
	}
	return s.result
}

func (s *Shared) tokens(source string) []parser.Token {
	if s == nil || !s.parsed || s.source != source {
		return parser.Tokenize(source)
	}
	return s.result.Tokens
}

// significantTokens returns the tokens of source without comments, worked out
// once per Shared and source. Read them, don't change them.
func (s *Shared) significantTokens(source string) []parser.Token {
	if s == nil {
		return parser.SignificantTokens(parser.Tokenize(source))
	}
	result := s.parse(source)
	if s.significant == nil {
		s.significant = parser.SignificantTokens(result.Tokens)
	}
	return s.significant
}
