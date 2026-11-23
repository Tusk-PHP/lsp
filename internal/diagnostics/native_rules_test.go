package diagnostics

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

func TestEveryRuleCodeIsEmittedByARule(t *testing.T) {
	rules := NativeRules(NativeRulesOptions{
		TypeResolver:           func(expr, source string, line, col int, file *parser.FileNode) string { return "" },
		BuiltinUnavailableRule: &checks.BuiltinUnavailableRule{},
		BuilderModelResolver:   func(prefix, method, source string, line int, file *parser.FileNode) string { return "" },
		BuilderMemberChecker:   &stubMemberChecker{},
	})
	emitted := map[rule.Code]bool{}
	for _, r := range rules {
		for _, code := range r.Codes() {
			emitted[code] = true
		}
	}
	for _, code := range blade.Codes {
		emitted[code] = true
	}
	for _, code := range rule.All {
		if !emitted[code] {
			t.Errorf("%q is declared in rule.All but no rule emits it", code)
		}
	}
	if len(emitted) != len(rule.All) {
		t.Errorf("rules emit %d codes, rule.All declares %d", len(emitted), len(rule.All))
	}
}

func TestEverySeverityHasAnLSPSeverity(t *testing.T) {
	for _, severity := range rule.Severities {
		if lspSeverity[severity] == 0 {
			t.Errorf("severity %s has no LSP severity", severity)
		}
	}
}

// TestNativeRulesWithoutDepsOmitsDependentRules ensures that rules requiring
// optional deps are silently omitted when those deps are nil.
func TestNativeRulesWithoutDepsOmitsDependentRules(t *testing.T) {
	rules := NativeRules(NativeRulesOptions{}) // no deps
	for _, r := range rules {
		switch r.Codes()[0] {
		case rule.UnknownMember, rule.RedundantNullsafe, rule.BuiltinUnavailable, rule.UnknownColumn:
			t.Errorf("rule %v should be omitted when no deps provided, but was included", r.Codes())
		}
	}
}

// TestNativeRulesDeterministicOrder ensures the parameterless rules always
// appear before the dep-requiring ones, in a stable order.
func TestNativeRulesDeterministicOrder(t *testing.T) {
	rules := NativeRules(NativeRulesOptions{})

	wantOrder := []rule.Code{
		rule.SyntaxError,
		rule.AbstractInConcrete,
		rule.UnknownClass,
		rule.UnknownFunction,
		rule.UnusedImport,
		rule.UnusedPrivateMethod,
		rule.UnreachableCode,
		rule.RedundantUnionMember,
		rule.UndefinedVariable,
		rule.UnusedVariable,
		rule.Deprecated,
		rule.Deprecated,
		rule.UnusedResult,
		rule.InvalidOverride,
		rule.ArgumentCountMismatch,
	}
	if len(rules) != len(wantOrder) {
		t.Fatalf("expected %d parameterless rules, got %d", len(wantOrder), len(rules))
	}
	for i, want := range wantOrder {
		if got := rules[i].Codes()[0]; got != want {
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
