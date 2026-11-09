package checks

import "github.com/Tusk-PHP/lsp/internal/parser"

// Shared holds what several rules run over the same file would each work
// out again: the file's tokens and structure. Make one per run over one
// version of a file and give it to each rule; a rule without one parses the
// file itself.
type Shared struct {
	source string
	result *parser.ParseResult
	parsed bool
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
