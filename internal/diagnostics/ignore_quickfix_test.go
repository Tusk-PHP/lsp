package diagnostics

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/protocol"
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
