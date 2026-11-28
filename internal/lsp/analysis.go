package lsp

import (
	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
)

// fileAnalysis is what the server asks the session about a file's
// diagnostics. Tests stand in for it to control when a pass finishes.
type fileAnalysis interface {
	Analyze(uri, source string) []diagnostics.Diagnostic
	Saved(uri, source string)
	RunTools(uri, filePath string, unchanged func() bool) bool
	Closed(uri string)
	NativeFindings(source string) []checks.Finding
}
