// Package checks provides standalone PHP static analysis rules that can be
// used by the LSP server, CLI tools, or CI pipelines. It has no dependency
// on LSP protocol types — all results are returned as Finding values.
package checks

import (
	"cmp"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Tag mirrors LSP DiagnosticTag values.
type Tag int

const (
	TagUnnecessary Tag = 1
	TagDeprecated  Tag = 2
)

// Finding represents a single diagnostic result.
type Finding struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	Severity  rule.Severity
	Code      rule.Code
	Message   string // human-readable description
	Tags      []Tag  // optional LSP diagnostic tags (Unnecessary, Deprecated)
}

func compareFindingStarts(a, b Finding) int {
	return cmp.Or(cmp.Compare(a.StartLine, b.StartLine), cmp.Compare(a.StartCol, b.StartCol))
}

// Rule is a single diagnostic check that can be enabled/disabled.
type Rule interface {
	Codes() []rule.Code
	Check(file *parser.FileNode, source string, index *symbols.Index) []Finding
}

// CheckFile runs all provided rules on a single file and returns the
// combined findings.
func CheckFile(file *parser.FileNode, source string, index *symbols.Index, rules []Rule) []Finding {
	var all []Finding
	for _, r := range rules {
		all = append(all, r.Check(file, source, index)...)
	}
	return all
}
