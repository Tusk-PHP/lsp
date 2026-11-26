package diagnostics

import (
	"log"
	"slices"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/config"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

type Provider struct {
	index     *symbols.Index
	framework string
	rootPath  string
	logger    *log.Logger
	cfg       *config.Config
	phpstan   *phpstanRunner
	pint      *pintRunner

	// TypeResolver resolves the type of a PHP expression for redundant nullsafe checks.
	// Set by the LSP server after initialization.
	TypeResolver checks.TypeResolver

	// BuilderModelResolver resolves the Eloquent model FQN from a builder chain.
	// Set by the LSP server after initialization.
	BuilderModelResolver checks.ModelResolverFunc

	// BuilderMemberChecker validates columns/relations on models.
	// Set by the LSP server after initialization.
	BuilderMemberChecker checks.MemberChecker

	Profile func() symbols.BuiltinProfile

	// Blade checks a Blade view's own structure and the views it names. Set
	// by the server after initialization.
	Blade *blade.Features
	// ViewTypes types the variables Blade views receive, and says when all
	// of them are known. Set by the server for a Laravel app.
	ViewTypes *frameworklaravel.ViewTypes

	mu          sync.RWMutex
	toolResults map[string][]protocol.Diagnostic
	saveResults map[string][]protocol.Diagnostic
}

func NewProvider(index *symbols.Index, framework, rootPath string, logger *log.Logger, cfg *config.Config) *Provider {
	p := &Provider{
		index:       index,
		framework:   framework,
		rootPath:    rootPath,
		logger:      logger,
		cfg:         cfg,
		toolResults: make(map[string][]protocol.Diagnostic),
		saveResults: make(map[string][]protocol.Diagnostic),
	}
	p.phpstan = newPHPStanRunner(rootPath, cfg, logger)
	p.pint = newPintRunner(rootPath, cfg, logger)
	return p
}

// Analyze runs fast static checks and merges cached external tool results.
// Called on every file change — only includes lightweight checks.
func (p *Provider) Analyze(uri, source string) []protocol.Diagnostic {
	return p.analyze(uri, source, false)
}

// AnalyzeAll returns what Analyze does plus the on-save rules' findings for
// this source, computed now and not cached: for one-shot runs over files on
// disk, such as `tusk-php check` and the MCP diagnostics tool, where every
// file counts as saved.
func (p *Provider) AnalyzeAll(uri, source string) []protocol.Diagnostic {
	return p.analyze(uri, source, true)
}

func (p *Provider) analyze(uri, source string, withSaveRules bool) []protocol.Diagnostic {
	var diags []protocol.Diagnostic
	shared := &checks.Shared{}
	if workspace.IsBlade(uri) {
		// A Blade view is checked through the PHP it contains, and only what
		// is found in that PHP is reported, at its place in the view.
		in := p.ViewTypes.For(uri)
		doc := blade.CompileWith(source, blade.CompileOptions{Vars: in.BladeVars(), Defined: in.Complete, Custom: p.bladeCustom()})
		diags = toTemplate(doc, p.analyzeNative(uri, doc.PHP, shared, true, !in.Complete))
		diags = append(diags, p.analyzeBlade(source)...)
	} else {
		diags = p.analyzeNative(uri, source, shared, false, workspace.IsTemplate(uri))
	}
	p.mu.RLock()
	if cached, ok := p.toolResults[uri]; ok {
		diags = append(diags, cached...)
	}
	if cached, ok := p.saveResults[uri]; ok && !withSaveRules {
		diags = append(diags, cached...)
	}
	p.mu.RUnlock()
	if withSaveRules {
		diags = append(diags, p.saveDiagnostics(uri, source)...)
	}

	// Soft-mode the unknown-* rules until the workspace + composer index has
	// completed at least once. The rules still ran (for testability), but their
	// findings depend on cross-file symbol presence and produce false positives
	// during the warm-up window.
	if p.index != nil && !p.index.Ready() {
		diags = filterOutCodes(diags, rule.UnknownClass, rule.UnknownFunction, rule.UnknownMember, rule.InvalidOverride)
	}

	return p.dropSuppressed(uri, source, shared, diags)
}

func (p *Provider) analyzeNative(uri, source string, shared *checks.Shared, bladePHP, includedFile bool) []protocol.Diagnostic {
	rules := EditRules(p.ruleOptions(bladePHP, includedFile), shared)
	return p.toDiagnostics(p.runRules(rules, parser.ParseFileIn(source, p.index.GrammarOf(uri)), source))
}

func (p *Provider) ruleOptions(bladePHP, includedFile bool) NativeRulesOptions {
	return NativeRulesOptions{
		TypeResolver:           p.TypeResolver,
		BuiltinUnavailableRule: p.BuiltinUnavailableRule(),
		BuilderModelResolver:   p.BuilderModelResolver,
		BuilderMemberChecker:   p.BuilderMemberChecker,
		BladePHP:               bladePHP,
		IncludedFile:           includedFile,
	}
}

func (p *Provider) BuiltinUnavailableRule() *checks.BuiltinUnavailableRule {
	if p.Profile == nil {
		return nil
	}
	profile := p.Profile()
	return &checks.BuiltinUnavailableRule{PHPVersion: profile.PHPVersion, Extensions: profile.Extensions}
}

func (p *Provider) runRules(rules []checks.Rule, file *parser.FileNode, source string) []checks.Finding {
	var findings []checks.Finding
	for _, r := range rules {
		if slices.ContainsFunc(r.Codes(), p.cfg.IsRuleEnabled) {
			findings = append(findings, r.Check(file, source, p.index)...)
		}
	}
	return findings
}

func (p *Provider) toDiagnostics(findings []checks.Finding) []protocol.Diagnostic {
	diags := make([]protocol.Diagnostic, 0, len(findings))
	for _, f := range findings {
		if !p.cfg.IsRuleEnabled(f.Code) {
			continue
		}
		f.Severity = p.cfg.RuleSeverity(f.Code, f.Severity)
		diags = append(diags, findingToDiagnostic(f))
	}
	return diags
}

// bladeCustom returns the directives and conditionals the project
// registers with Blade.
func (p *Provider) bladeCustom() *blade.Custom {
	if p.Blade == nil || p.Blade.Custom == nil {
		return nil
	}
	return p.Blade.Custom()
}

func (p *Provider) analyzeBlade(source string) []protocol.Diagnostic {
	if p.Blade == nil {
		return nil
	}
	found := p.Blade.Diagnose(source)
	if len(found) == 0 {
		return nil
	}
	lines := blade.NewLines(source)
	findings := make([]checks.Finding, 0, len(found))
	for _, d := range found {
		sl, sc := lines.Position(d.Span.Start)
		el, ec := lines.Position(d.Span.End)
		findings = append(findings, checks.Finding{
			StartLine: sl, StartCol: sc, EndLine: el, EndCol: ec,
			Severity: d.Severity, Code: d.Code, Message: d.Message,
		})
	}
	return p.toDiagnostics(findings)
}

// toTemplate moves diagnostics found in a Blade view's virtual PHP to the
// view. Those that start in generated PHP have no place in the view and are
// dropped.
func toTemplate(doc *blade.Document, diags []protocol.Diagnostic) []protocol.Diagnostic {
	out := diags[:0]
	for _, d := range diags {
		r := d.Range
		sl, sc, el, ec, ok := doc.TemplateRange(r.Start.Line, r.Start.Character, r.End.Line, r.End.Character)
		if !ok {
			continue
		}
		d.Range = protocol.Range{Start: protocol.Position{Line: sl, Character: sc}, End: protocol.Position{Line: el, Character: ec}}
		out = append(out, d)
	}
	return out
}

// AnalyzeOnSave runs heavier checks that need type resolution or model lookups.
// Called on file save only. Results are cached and merged into Analyze() output.
func (p *Provider) AnalyzeOnSave(uri, source string) {
	diags := p.saveDiagnostics(uri, source)
	p.mu.Lock()
	p.saveResults[uri] = diags
	p.mu.Unlock()
}

// saveDiagnostics runs the rules that wait for a save.
func (p *Provider) saveDiagnostics(uri, source string) []protocol.Diagnostic {
	var doc *blade.Document
	if workspace.IsBlade(uri) {
		doc = blade.CompileWith(source, blade.CompileOptions{Vars: p.ViewTypes.For(uri).BladeVars(), Custom: p.bladeCustom()})
		source = doc.PHP
	}
	diags := p.toDiagnostics(p.runRules(SaveRules(p.ruleOptions(doc != nil, false)), parser.ParseFileIn(source, p.index.GrammarOf(uri)), source))
	if doc != nil {
		diags = toTemplate(doc, diags)
	}
	return diags
}

// RunTools executes external analysis tools (PHPStan, Pint) on a file.
// Results are cached and included in subsequent Analyze calls.
func (p *Provider) RunTools(uri, filePath string) {
	var toolDiags []protocol.Diagnostic
	if p.phpstan.available() {
		toolDiags = append(toolDiags, p.phpstan.analyze(filePath)...)
	}
	if p.pint.available() {
		toolDiags = append(toolDiags, p.pint.analyze(filePath)...)
	}
	p.mu.Lock()
	p.toolResults[uri] = toolDiags
	p.mu.Unlock()
}

// ClearCache removes cached tool results for a URI.
func (p *Provider) ClearCache(uri string) {
	p.mu.Lock()
	delete(p.toolResults, uri)
	delete(p.saveResults, uri)
	p.mu.Unlock()
}

// filterOutCodes returns a new slice with all diagnostics whose Code matches
// one of the provided codes removed. It reuses the backing array of diags when
// possible to avoid an allocation in the common (no-op) case where the slice is
// already empty.
func filterOutCodes(diags []protocol.Diagnostic, codes ...rule.Code) []protocol.Diagnostic {
	if len(diags) == 0 {
		return diags
	}
	skip := make(map[string]bool, len(codes))
	for _, c := range codes {
		skip[string(c)] = true
	}
	out := diags[:0]
	for _, d := range diags {
		if !skip[d.Code] {
			out = append(out, d)
		}
	}
	return out
}

var lspSeverity = map[rule.Severity]protocol.DiagnosticSeverity{
	rule.SeverityError:   protocol.DiagnosticSeverityError,
	rule.SeverityWarning: protocol.DiagnosticSeverityWarning,
	rule.SeverityInfo:    protocol.DiagnosticSeverityInformation,
	rule.SeverityHint:    protocol.DiagnosticSeverityHint,
}

func findingToDiagnostic(f checks.Finding) protocol.Diagnostic {
	d := protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: f.StartLine, Character: f.StartCol},
			End:   protocol.Position{Line: f.EndLine, Character: f.EndCol},
		},
		Severity: lspSeverity[f.Severity],
		Source:   string(rule.SourceTusk),
		Message:  f.Message,
		Code:     string(f.Code),
	}
	for _, tag := range f.Tags {
		d.Tags = append(d.Tags, protocol.DiagnosticTag(tag))
	}
	return d
}
