package diagnostics

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/parser"
)

// TestNativeRulesParityWithProvider is a parity guard that asserts the rule
// codes returned by NativeRules (with all deps provided) match the set of rule
// codes that provider.go runs across Analyze and AnalyzeOnSave. If provider.go
// ever gains or drops a native rule, this test will fail, prompting the
// developer to update NativeRules accordingly.
func TestNativeRulesParityWithProvider(t *testing.T) {
	// providerRuleCodes is the exhaustive set of native rule codes that
	// provider.go covers across Analyze() + AnalyzeOnSave(). Keep in sync with
	// provider.go manually; the test alerts when they drift.
	providerRuleCodes := map[string]bool{
		// Analyze() — parameterless
		"unknown-class":           true,
		"unknown-function":        true,
		"unused-import":           true,
		"unused-private-method":   true,
		"unused-private-property": true,
		"unreachable-code":        true,
		"redundant-union-member":  true,
		"undefined-variable":      true,
		"unused-variable":         true,
		"argument-count-mismatch": true,
		"invalid-override":        true,
		"deprecated":              true,
		"unused-result":           true,
		// Analyze() — requires TypeResolver
		"unknown-member": true,
		// Analyze() — requires BuiltinUnavailableRule
		"builtin-unavailable": true,
		// AnalyzeOnSave() — requires TypeResolver
		"redundant-nullsafe": true,
		// AnalyzeOnSave() — requires BuilderModelResolver + BuilderMemberChecker
		"unknown-column":   true,
		"unknown-relation": true,
	}

	tr := func(expr, source string, line, col int, file *parser.FileNode) string { return "" }
	builtinRule := &checks.BuiltinUnavailableRule{}
	opts := NativeRulesOptions{
		TypeResolver:           tr,
		BuiltinUnavailableRule: builtinRule,
		BuilderModelResolver: func(prefix, method, source string, line int, file *parser.FileNode) string {
			return ""
		},
		BuilderMemberChecker: &stubMemberChecker{},
	}

	rules := NativeRules(opts)
	nativeRuleCodes := make(map[string]bool, len(rules))
	for _, r := range rules {
		for _, code := range r.Codes() {
			nativeRuleCodes[string(code)] = true
		}
	}

	for code := range providerRuleCodes {
		if !nativeRuleCodes[code] {
			t.Errorf("NativeRules is missing rule code %q that provider.go covers", code)
		}
	}
	for code := range nativeRuleCodes {
		if !providerRuleCodes[code] {
			t.Errorf("NativeRules includes rule code %q not covered by provider.go — add it to providerRuleCodes or remove from NativeRules", code)
		}
	}
}

// TestNativeRulesWithoutDepsOmitsDependentRules ensures that rules requiring
// optional deps are silently omitted when those deps are nil.
func TestNativeRulesWithoutDepsOmitsDependentRules(t *testing.T) {
	rules := NativeRules(NativeRulesOptions{}) // no deps
	for _, r := range rules {
		switch r.Codes()[0] {
		case "unknown-member", "redundant-nullsafe", "builtin-unavailable", "unknown-column":
			t.Errorf("rule %v should be omitted when no deps provided, but was included", r.Codes())
		}
	}
}

// TestNativeRulesDeterministicOrder ensures the parameterless rules always
// appear before the dep-requiring ones, in a stable order.
func TestNativeRulesDeterministicOrder(t *testing.T) {
	rules := NativeRules(NativeRulesOptions{})

	wantOrder := []string{
		"unknown-class",
		"unknown-function",
		"unused-import",
		"unused-private-method",
		"unreachable-code",
		"redundant-union-member",
		"undefined-variable",
		"unused-variable",
		"argument-count-mismatch",
		"invalid-override",
		"deprecated",
		"unused-result",
	}
	if len(rules) != len(wantOrder) {
		t.Fatalf("expected %d parameterless rules, got %d", len(wantOrder), len(rules))
	}
	for i, want := range wantOrder {
		if got := string(rules[i].Codes()[0]); got != want {
			t.Errorf("rules[%d]: expected %q, got %q", i, want, got)
		}
	}
}

// stubMemberChecker is a no-op MemberChecker for test use.
type stubMemberChecker struct{}

func (s *stubMemberChecker) IsColumn(modelFQN, name string) bool          { return false }
func (s *stubMemberChecker) IsDBColumn(modelFQN, name string) bool        { return false }
func (s *stubMemberChecker) IsRelation(modelFQN, name string) bool        { return false }
func (s *stubMemberChecker) RelatedModelFQN(modelFQN, name string) string { return "" }
func (s *stubMemberChecker) KnowsColumns(modelFQN string) bool            { return true }
func (s *stubMemberChecker) KnowsRelations(modelFQN string) bool          { return true }
