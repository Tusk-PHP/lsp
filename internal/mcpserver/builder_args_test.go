package mcpserver

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// TestRuntimeWiresBuilderArgRules checks that the MCP runtime gives the
// diagnostics provider what the unknown-relation and unknown-column rules
// need, and that they then run on the Laravel fixture.
func TestRuntimeWiresBuilderArgRules(t *testing.T) {
	svc := sharedLaravelSvc(t)
	rt := svc.runtime()
	if rt.Diagnostics.BuilderModelResolver == nil || rt.Diagnostics.BuilderMemberChecker == nil {
		t.Fatal("the builder model resolver or member checker isn't wired")
	}
	const uri = "file:///probe/RelationProbe.php"
	src := `<?php
use App\Models\Product;
Product::with(['category', 'missingRelation'])->get();
Product::where('missing_column', 1)->get();
`
	rt.Diagnostics.AnalyzeOnSave(uri, src)
	defer rt.Diagnostics.ClearCache(uri)
	var got []string
	for _, d := range rt.Diagnostics.Analyze(uri, src) {
		if d.Code == "unknown-relation" || d.Code == "unknown-column" {
			got = append(got, string(d.Code)+": "+d.Message)
			if d.Severity != protocol.DiagnosticSeverityWarning {
				t.Errorf("%s: severity %d", d.Code, d.Severity)
			}
		}
	}
	// The fixture's columns come from migrations, so columns aren't judged.
	if want := "unknown-relation: Unknown relation 'missingRelation' on model 'Product'"; strings.Join(got, "\n") != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
