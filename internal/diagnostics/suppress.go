package diagnostics

import (
	"path/filepath"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func (p *Provider) dropSuppressed(uri, source string, shared *checks.Shared, diags []Diagnostic) []Diagnostic {
	if len(diags) == 0 {
		return diags
	}
	rules := p.ignoreRulesFor(uri)
	suppressions := p.sourceSuppressions(uri, source, shared)
	if len(rules) == 0 && suppressions == nil {
		return diags
	}
	kept := make([]Diagnostic, 0, len(diags))
	for _, d := range diags {
		if ignoredByRule(rules, d) || suppressions.Suppresses(d.Range.Start.Line, d.Source, d.Code) {
			continue
		}
		kept = append(kept, d)
	}
	return kept
}

func (p *Provider) sourceSuppressions(uri, source string, shared *checks.Shared) checks.Suppressions {
	if workspace.IsBlade(uri) {
		return checks.BladeSuppressions(source, p.bladeCustom())
	}
	return checks.PHPSuppressions(source, shared)
}

func (p *Provider) ignoreRulesFor(uri string) []config.IgnoreRule {
	if len(p.cfg.DiagnosticIgnore.Rules) == 0 {
		return nil
	}
	rel, inRoot := p.relativePath(uri)
	var rules []config.IgnoreRule
	for _, r := range p.cfg.DiagnosticIgnore.Rules {
		if !r.HasPath() || (inRoot && r.MatchesPath(rel)) {
			rules = append(rules, r)
		}
	}
	return rules
}

func (p *Provider) relativePath(uri string) (string, bool) {
	rel, err := filepath.Rel(p.rootPath, symbols.URIToPath(uri))
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	return rel, rel != ".." && !strings.HasPrefix(rel, "../")
}

func ignoredByRule(rules []config.IgnoreRule, d Diagnostic) bool {
	for _, rule := range rules {
		if rule.Matches(d.Source, d.Code, d.Message) {
			return true
		}
	}
	return false
}
