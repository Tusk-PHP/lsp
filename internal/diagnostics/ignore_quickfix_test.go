package diagnostics

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

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
	p := newTestProvider()
	p.index.MarkReady()
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
			Context:      protocol.CodeActionContext{Diagnostics: diags[:1]},
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
	p := NewProvider(idx, "laravel", "/tmp", nil, config.DefaultConfig())
	uri := "file:///tmp/resources/views/rows.blade.php"
	source := "<table>\n    @include('row', [\n        'total' => missing(),\n    ])\n</table>\n"
	diags := p.Analyze(uri, source)
	if len(diags) != 1 || diags[0].Range.Start.Line != 2 {
		t.Fatalf("missing() is reported on the directive's second line, got %+v", diags)
	}
	params := protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Context:      protocol.CodeActionContext{Diagnostics: diags},
	}
	actions := analyzer.IgnoreDiagnosticActions(uri, source, params)
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
	if again := analyzer.IgnoreDiagnosticActions(uri, fixed, params); len(again) != 0 {
		t.Errorf("asked again with the finding it already silences, the fix adds nothing, got %+v", again)
	}
}
