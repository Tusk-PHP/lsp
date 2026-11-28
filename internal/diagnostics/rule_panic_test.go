package diagnostics

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/strict"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type panickingRule struct{}

func (panickingRule) Codes() []rule.Code { return []rule.Code{rule.UnusedImport} }

func (panickingRule) Check(*parser.FileNode, string, *symbols.Index) []checks.Finding {
	panic("rule exploded")
}

type findingRule struct{}

func (findingRule) Codes() []rule.Code { return []rule.Code{rule.UnreachableCode} }

func (findingRule) Check(*parser.FileNode, string, *symbols.Index) []checks.Finding {
	return []checks.Finding{{Code: rule.UnreachableCode, Message: "still here"}}
}

func panicTestProvider() (*Provider, *bytes.Buffer) {
	var logs bytes.Buffer
	return NewProvider(symbols.NewIndex(), "none", "/tmp", log.New(&logs, "", 0), config.DefaultConfig()), &logs
}

func TestAPanickingRuleKeepsTheOtherRulesFindings(t *testing.T) {
	t.Setenv(strict.EnvVar, "")
	p, logs := panicTestProvider()
	source := "<?php\n"
	got := p.runRules("file:///app/a.php", []checks.Rule{findingRule{}, panickingRule{}, findingRule{}}, parser.ParseFile(source), source)
	if len(got) != 2 || got[0].Message != "still here" || got[1].Message != "still here" {
		t.Fatalf("got %+v, want the two findings of the rules that didn't panic", got)
	}
	for _, want := range []string{"unused-import", "file:///app/a.php", "rule exploded", "runtime/debug.Stack"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("the log doesn't mention %q:\n%s", want, logs.String())
		}
	}
}

func TestAPanickingRulePanicsInStrictMode(t *testing.T) {
	p, logs := panicTestProvider()
	t.Setenv(strict.EnvVar, "1")
	defer func() {
		if r := recover(); r != "rule exploded" {
			t.Errorf("recovered %v, want the rule's panic", r)
		}
		if !strings.Contains(logs.String(), "rule exploded") {
			t.Errorf("the panic wasn't logged before it was raised again:\n%s", logs.String())
		}
	}()
	source := "<?php\n"
	p.runRules("file:///app/a.php", []checks.Rule{panickingRule{}}, parser.ParseFile(source), source)
	t.Error("runRules returned instead of panicking")
}
