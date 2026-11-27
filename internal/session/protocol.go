package session

import (
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// LSPDiagnostics gives the engine's diagnostics the LSP's shape, which the
// language server publishes and the MCP diagnostics tool returns. Columns
// stay in bytes: a transport converts them to its client's unit. The result
// is never nil, so a clean file is an empty list. rule.Severity and
// checks.Tag use the LSP's numbers, so they carry over.
func LSPDiagnostics(diags []diagnostics.Diagnostic) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(diags))
	for _, d := range diags {
		pd := protocol.Diagnostic{
			Range:    d.Range,
			Severity: protocol.DiagnosticSeverity(d.Severity),
			Source:   d.Source,
			Code:     d.Code,
			Message:  d.Message,
		}
		for _, tag := range d.Tags {
			pd.Tags = append(pd.Tags, protocol.DiagnosticTag(tag))
		}
		out = append(out, pd)
	}
	return out
}
