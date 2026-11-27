package diagnostics_test

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// lspDiagnostics is what the language server sends for diags, as the quick
// fixes receive them back.
func lspDiagnostics(diags []diagnostics.Diagnostic) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(diags))
	for _, d := range diags {
		pd := protocol.Diagnostic{Range: d.Range, Severity: protocol.DiagnosticSeverity(d.Severity), Source: d.Source, Code: d.Code, Message: d.Message}
		for _, tag := range d.Tags {
			pd.Tags = append(pd.Tags, protocol.DiagnosticTag(tag))
		}
		out = append(out, pd)
	}
	return out
}

func applyInsertion(t *testing.T, source string, edit protocol.TextEdit) string {
	t.Helper()
	if edit.Range.Start != edit.Range.End {
		t.Fatalf("the quick fix only inserts, got %+v", edit)
	}
	lines := strings.SplitAfter(source, "\n")
	line := lines[edit.Range.Start.Line]
	lines[edit.Range.Start.Line] = line[:edit.Range.Start.Character] + edit.NewText + line[edit.Range.Start.Character:]
	return strings.Join(lines, "")
}

func TestIgnoreQuickFixSilencesFindingsOnAttributeLines(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.MarkReady()
	cfg := config.DefaultConfig()
	off := false
	cfg.PHPStanEnabled, cfg.PintEnabled = &off, &off
	p := diagnostics.NewProvider(idx, "none", "/tmp", nil, cfg)
	uri := "file:///tmp/Shop.php"
	source := `<?php
class Shop
{
    /**
     * @return int
     */
    #[\NoSuchAttr]
    public function total(): int { return 1; }
}
`
	diags := p.Analyze(uri, source)
	if len(diags) == 0 {
		t.Fatal("the unknown attribute class is reported")
	}
	for len(diags) > 0 {
		actions := analyzer.IgnoreDiagnosticActions(uri, source, protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Context:      protocol.CodeActionContext{Diagnostics: lspDiagnostics(diags[:1])},
		})
		if len(actions) == 0 {
			t.Fatalf("no quick fix for %+v", diags[0])
		}
		source = applyInsertion(t, source, actions[0].Edit.Changes[uri][0])
		next := p.Analyze(uri, source)
		if len(next) >= len(diags) {
			t.Fatalf("applying %q left %+v in\n%s", actions[0].Title, next, source)
		}
		diags = next
	}
	if !strings.Contains(source, "    // @tusk-ignore unknown-class\n    /**") {
		t.Errorf("the marker goes above the docblock:\n%s", source)
	}
}

func TestIgnoreQuickFixSilencesMultiLineBladeDirectives(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.MarkReady()
	p := diagnostics.NewProvider(idx, "laravel", "/tmp", nil, config.DefaultConfig())
	uri := "file:///tmp/resources/views/rows.blade.php"
	source := "<table>\n    @include('row', [\n        'total' => missing(),\n    ])\n</table>\n"
	diags := p.Analyze(uri, source)
	if len(diags) != 1 || diags[0].Range.Start.Line != 2 {
		t.Fatalf("missing() is reported on the directive's second line, got %+v", diags)
	}
	params := protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Context:      protocol.CodeActionContext{Diagnostics: lspDiagnostics(diags)},
	}
	actions := analyzer.BladeIgnoreDiagnosticActions(uri, source, params, nil)
	if len(actions) != 1 {
		t.Fatalf("one quick fix, got %+v", actions)
	}
	fixed := applyInsertion(t, source, actions[0].Edit.Changes[uri][0])
	want := "<table>\n    {{-- @tusk-ignore unknown-function --}}\n    @include('row', [\n        'total' => missing(),\n    ])\n</table>\n"
	if fixed != want {
		t.Fatalf("the marker goes above the directive:\n%s", fixed)
	}
	after := p.Analyze(uri, fixed)
	if len(after) != 0 {
		t.Fatalf("the finding is gone after the fix, got %+v", after)
	}
	if again := analyzer.BladeIgnoreDiagnosticActions(uri, fixed, params, nil); len(again) != 0 {
		t.Errorf("asked again with the finding it already silences, the fix adds nothing, got %+v", again)
	}
}

func TestIgnoreQuickFixSilencesMultiLineCustomBladeConditionals(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.MarkReady()
	p := diagnostics.NewProvider(idx, "laravel", "/tmp", nil, config.DefaultConfig())
	custom := &blade.Custom{Conditionals: map[string]bool{"admin": true}}
	p.Blade = &blade.Features{Custom: func() *blade.Custom { return custom }}
	uri := "file:///tmp/resources/views/panel.blade.php"
	source := "<div>\n    @admin([\n        'role' => missing(),\n    ])\n        x\n    @endadmin\n</div>\n"
	diags := p.Analyze(uri, source)
	if len(diags) != 1 || diags[0].Range.Start.Line != 2 {
		t.Fatalf("missing() is reported on the conditional's second line, got %+v", diags)
	}
	params := protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Context:      protocol.CodeActionContext{Diagnostics: lspDiagnostics(diags)},
	}
	actions := analyzer.BladeIgnoreDiagnosticActions(uri, source, params, custom)
	if len(actions) != 1 {
		t.Fatalf("one quick fix, got %+v", actions)
	}
	fixed := applyInsertion(t, source, actions[0].Edit.Changes[uri][0])
	want := "<div>\n    {{-- @tusk-ignore unknown-function --}}\n    @admin([\n        'role' => missing(),\n    ])\n        x\n    @endadmin\n</div>\n"
	if fixed != want {
		t.Fatalf("the marker goes above the conditional:\n%s", fixed)
	}
	if after := p.Analyze(uri, fixed); len(after) != 0 {
		t.Fatalf("the finding is gone after the fix, got %+v", after)
	}
}
