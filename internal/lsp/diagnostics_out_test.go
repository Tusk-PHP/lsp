package lsp

import (
	"reflect"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// lspDiagnostics casts severities and tags, so their numbers must be the
// LSP's.
func TestRuleSeveritiesAndTagsAreTheLSPs(t *testing.T) {
	for severity, want := range map[rule.Severity]protocol.DiagnosticSeverity{
		rule.SeverityError:   protocol.DiagnosticSeverityError,
		rule.SeverityWarning: protocol.DiagnosticSeverityWarning,
		rule.SeverityInfo:    protocol.DiagnosticSeverityInformation,
		rule.SeverityHint:    protocol.DiagnosticSeverityHint,
	} {
		if protocol.DiagnosticSeverity(severity) != want {
			t.Errorf("severity %s is %d, the LSP's is %d", severity, severity, want)
		}
	}
	if len(rule.Severities) != 4 {
		t.Errorf("a new severity needs an LSP severity: %v", rule.Severities)
	}
	if protocol.DiagnosticTag(checks.TagUnnecessary) != protocol.DiagnosticTagUnnecessary || protocol.DiagnosticTag(checks.TagDeprecated) != protocol.DiagnosticTagDeprecated {
		t.Error("checks.Tag values aren't the LSP's")
	}
}

func TestLSPDiagnostics(t *testing.T) {
	r := textpos.Range{Start: textpos.Position{Line: 1, Character: 2}, End: textpos.Position{Line: 1, Character: 7}}
	got := lspDiagnostics([]diagnostics.Diagnostic{{
		Range: r, Severity: rule.SeverityHint, Source: string(rule.SourceTusk), Code: "unused-variable", Message: "unused", Tags: []checks.Tag{checks.TagUnnecessary},
	}})
	want := []protocol.Diagnostic{{
		Range: r, Severity: protocol.DiagnosticSeverityHint, Source: string(rule.SourceTusk), Code: "unused-variable", Message: "unused", Tags: []protocol.DiagnosticTag{protocol.DiagnosticTagUnnecessary},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got := lspDiagnostics(nil); got == nil || len(got) != 0 {
		t.Errorf("no diagnostics publish an empty list, got %#v", got)
	}
}
