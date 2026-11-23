package diagnostics

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// providerWithRules returns a test provider configured with the given
// diagnosticRules JSON.
func providerWithRules(t *testing.T, rules string) *Provider {
	t.Helper()
	p := newTestProvider()
	if err := p.cfg.SetDiagnosticRulesJSON(rules); err != nil {
		t.Fatal(err)
	}
	return p
}

const redundantUnionSource = `<?php
function f(int|string|int $x): void {}
`

func TestRedundantUnionMemberDefaultSeverity(t *testing.T) {
	got := filterByCode(newTestProvider().Analyze("file:///t.php", redundantUnionSource), "redundant-union-member")
	if len(got) != 1 {
		t.Fatalf("expected 1 redundant-union-member, got %d", len(got))
	}
	if got[0].Severity != protocol.DiagnosticSeverityInformation {
		t.Errorf("default severity: want information, got %d", got[0].Severity)
	}
}

// TestRuleSeverityConfigApplies checks that a configured severity applies
// even when it is the rule's registered default and differs from the
// severity the rule gave its finding.
func TestRuleSeverityConfigApplies(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  protocol.DiagnosticSeverity
	}{
		{`"warning"`, protocol.DiagnosticSeverityWarning},
		{`"error"`, protocol.DiagnosticSeverityError},
		{`"hint"`, protocol.DiagnosticSeverityHint},
	} {
		p := providerWithRules(t, `{"redundant-union-member": `+tc.value+`}`)
		got := filterByCode(p.Analyze("file:///t.php", redundantUnionSource), "redundant-union-member")
		if len(got) != 1 || got[0].Severity != tc.want {
			t.Errorf("%s: want one diagnostic with severity %d, got %#v", tc.value, tc.want, got)
		}
	}
}

func TestSyntaxErrorRuleIsConfigurable(t *testing.T) {
	source := `<?php
$name = "unterminated;
`
	off := providerWithRules(t, `{"syntax-error": false}`)
	if got := filterByCode(off.Analyze("file:///t.php", source), "syntax-error"); len(got) != 0 {
		t.Errorf("syntax-error turned off: got %d diagnostics", len(got))
	}
	warn := providerWithRules(t, `{"syntax-error": "warning"}`)
	got := filterByCode(warn.Analyze("file:///t.php", source), "syntax-error")
	if len(got) != 1 || got[0].Severity != protocol.DiagnosticSeverityWarning {
		t.Errorf("syntax-error as warning: got %#v", got)
	}
}

func TestAbstractInConcreteRuleIsConfigurable(t *testing.T) {
	source := `<?php
class Foo {
    abstract public function bar(): void;
}
`
	if got := filterByCode(newTestProvider().Analyze("file:///t.php", source), "abstract-in-concrete"); len(got) != 1 || got[0].Severity != protocol.DiagnosticSeverityError {
		t.Fatalf("default: want one error, got %#v", got)
	}
	off := providerWithRules(t, `{"abstract-in-concrete": false}`)
	if got := filterByCode(off.Analyze("file:///t.php", source), "abstract-in-concrete"); len(got) != 0 {
		t.Errorf("turned off: got %d diagnostics", len(got))
	}
	info := providerWithRules(t, `{"abstract-in-concrete": "info"}`)
	if got := filterByCode(info.Analyze("file:///t.php", source), "abstract-in-concrete"); len(got) != 1 || got[0].Severity != protocol.DiagnosticSeverityInformation {
		t.Errorf("as info: got %#v", got)
	}
}

// missingMembers knows one model with no columns and no relations.
type missingMembers struct{ stubMemberChecker }

const builderSource = `<?php
use App\Models\User;
User::where('missing_column', 1)->with('missingRelation')->get();
`

func builderProvider(t *testing.T, rules string) *Provider {
	p := providerWithRules(t, rules)
	p.BuilderModelResolver = func(prefix, method, source string, line int, file *parser.FileNode) string {
		return `App\Models\User`
	}
	p.BuilderMemberChecker = &missingMembers{}
	return p
}

func TestBuilderRulesAreConfigurable(t *testing.T) {
	const uri = "file:///t.php"
	p := builderProvider(t, `{}`)
	p.AnalyzeOnSave(uri, builderSource)
	diags := p.Analyze(uri, builderSource)
	cols, rels := filterByCode(diags, "unknown-column"), filterByCode(diags, "unknown-relation")
	if len(cols) != 1 || len(rels) != 1 {
		t.Fatalf("defaults: want one unknown-column and one unknown-relation, got %d and %d", len(cols), len(rels))
	}
	if cols[0].Severity != protocol.DiagnosticSeverityWarning || rels[0].Severity != protocol.DiagnosticSeverityWarning {
		t.Errorf("defaults: want warnings, got %d and %d", cols[0].Severity, rels[0].Severity)
	}

	p = builderProvider(t, `{"unknown-column": "error", "unknown-relation": false}`)
	p.AnalyzeOnSave(uri, builderSource)
	diags = p.Analyze(uri, builderSource)
	cols, rels = filterByCode(diags, "unknown-column"), filterByCode(diags, "unknown-relation")
	if len(cols) != 1 || cols[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("unknown-column as error: got %#v", cols)
	}
	if len(rels) != 0 {
		t.Errorf("unknown-relation turned off: got %d diagnostics", len(rels))
	}
}
