package mcpserver

import (
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// lspDiagnostics gives the engine's diagnostics the LSP's shape, which the
// diagnostics tool has always returned, with byte columns. nil stays nil.
// rule.Severity and checks.Tag use the LSP's numbers, so they carry over.
func lspDiagnostics(diags []diagnostics.Diagnostic) []protocol.Diagnostic {
	if diags == nil {
		return nil
	}
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
