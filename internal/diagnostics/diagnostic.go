package diagnostics

import (
	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// Diagnostic is one problem found in a file, from a native rule, PHPStan or
// Pint. The language server, the MCP server and the CLI turn it into their
// own output; its Range is in byte columns, like every engine position.
type Diagnostic struct {
	Range    textpos.Range
	Severity rule.Severity
	// Source is who reported it: rule.SourceTusk, rule.SourcePHPStan or
	// rule.SourcePint.
	Source string
	// Code is the rule code for a native rule, PHPStan's error identifier,
	// or the Pint fixer.
	Code    string
	Message string
	Tags    []checks.Tag
}
