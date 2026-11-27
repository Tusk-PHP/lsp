package mcpserver

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/rule"
)

// TestRuntimeWiresBuilderArgRules checks that the MCP runtime's session
// runs the unknown-relation and unknown-column rules on the Laravel fixture.
func TestRuntimeWiresBuilderArgRules(t *testing.T) {
	svc := sharedLaravelSvc(t)
	rt := svc.runtime()
	const uri = "file:///probe/RelationProbe.php"
	src := `<?php
use App\Models\Product;
Product::with(['category', 'missingRelation'])->get();
Product::where('missing_column', 1)->get();
`
	rt.Session.Saved(uri, src)
	defer rt.Session.Closed(uri)
	var got []string
	for _, d := range rt.Session.Analyze(uri, src) {
		if d.Code == "unknown-relation" || d.Code == "unknown-column" {
			got = append(got, string(d.Code)+": "+d.Message)
			if d.Severity != rule.SeverityWarning {
				t.Errorf("%s: severity %d", d.Code, d.Severity)
			}
		}
	}
	// The fixture's columns come from migrations, so columns aren't judged.
	if want := "unknown-relation: Unknown relation 'missingRelation' on model 'Product'"; strings.Join(got, "\n") != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
