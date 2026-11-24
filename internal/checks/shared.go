package checks

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
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
	syntax *fileSyntax
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

func (s *Shared) tokens(source string) []parser.Token {
	if s == nil || !s.parsed || s.source != source {
		return parser.Tokenize(source)
	}
	return s.result.Tokens
}

func (s *Shared) tree(source string, grammar token.Version) *fileSyntax {
	if s == nil {
		return newFileSyntax(source, grammar)
	}
	if s.syntax == nil || s.syntax.src != source || s.syntax.tree.Version != grammar {
		s.syntax = newFileSyntax(source, grammar)
	}
	return s.syntax
}

type fileSyntax struct {
	src   string
	tree  *ast.File
	lines ast.Lines
	refs  *references
}

func (f *fileSyntax) references() *references {
	if f.refs == nil {
		f.refs = collectReferences(f.tree)
	}
	return f.refs
}

func (f *fileSyntax) line(n ast.Node) int {
	line, _ := f.lines.Position(n.Bounds().Start)
	return line
}

func newFileSyntax(source string, grammar token.Version) *fileSyntax {
	return &fileSyntax{src: source, tree: parser.ParseTree(source, grammar), lines: ast.NewLines(source)}
}

func (f *fileSyntax) text(n ast.Node) string {
	return n.Bounds().Text(f.src)
}

func (f *fileSyntax) at(n ast.Node, finding Finding) Finding {
	span := n.Bounds()
	finding.StartLine, finding.StartCol = f.lines.Position(span.Start)
	finding.EndLine, finding.EndCol = f.lines.Position(span.End)
	return finding
}
