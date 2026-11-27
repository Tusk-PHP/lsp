package lsp

import (
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// lspDiagnostics turns the engine's diagnostics into the LSP's, with byte
// columns still: positions are converted to the client's unit afterwards.
// rule.Severity and checks.Tag use the LSP's numbers, so they carry over.
func lspDiagnostics(diags []diagnostics.Diagnostic) []protocol.Diagnostic {
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
