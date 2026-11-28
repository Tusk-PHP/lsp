package diagnostics

import (
	"log"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/config"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

type Provider struct {
	index     *symbols.Index
	framework string
	rootPath  string
	logger    *log.Logger
	settings  atomic.Pointer[settings]

	// TypeResolver resolves the type of a PHP expression for redundant nullsafe checks.
	// Set by the LSP server after initialization.
	TypeResolver func(expr string, source string, line, col int, file *parser.FileNode) string

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
	toolResults map[string][]Diagnostic
	saveResults map[string]savedFindings
	saveGen     uint64
}

type savedFindings struct {
	source   string
	diags    []Diagnostic
	gen      uint64
	settings *settings
}

func NewProvider(index *symbols.Index, framework, rootPath string, logger *log.Logger, cfg *config.Config) *Provider {
	p := &Provider{
		index:       index,
		framework:   framework,
		rootPath:    rootPath,
		logger:      logger,
		toolResults: make(map[string][]Diagnostic),
		saveResults: make(map[string]savedFindings),
	}
	p.SetConfig(cfg)
	return p
}

type settings struct {
	cfg     *config.Config
	phpstan *phpstanRunner
	pint    *pintRunner
}

func (s *settings) sameTools(other *settings) bool {
	return *s.phpstan == *other.phpstan && *s.pint == *other.pint
}

func (p *Provider) SetConfig(cfg *config.Config) {
	next := &settings{cfg: cfg, phpstan: newPHPStanRunner(p.rootPath, cfg, p.logger), pint: newPintRunner(p.rootPath, cfg, p.logger)}
	prev := p.settings.Swap(next)
	if prev == nil || prev.sameTools(next) {
		return
	}
	p.mu.Lock()
	clear(p.toolResults)
	p.mu.Unlock()
}

// Analyze runs fast static checks and merges cached external tool results.
// Called on every file change — only includes lightweight checks.
func (p *Provider) Analyze(uri, source string) []Diagnostic {
	return p.analyze(uri, source, false)
}

// AnalyzeAll returns what Analyze does plus the on-save rules' findings for
// this source, computed now and not cached: for one-shot runs over files on
// disk, such as `tusk-php check` and the MCP diagnostics tool, where every
// file counts as saved.
func (p *Provider) AnalyzeAll(uri, source string) []Diagnostic {
	return p.analyze(uri, source, true)
}

func (p *Provider) analyze(uri, source string, withSaveRules bool) []Diagnostic {
	cfg := p.settings.Load().cfg
	var diags []Diagnostic
	shared := &checks.Shared{}
	if workspace.IsBlade(uri) {
		// A Blade view is checked through the PHP it contains, and only what
		// is found in that PHP is reported, at its place in the view.
		in := p.ViewTypes.For(uri)
		doc := blade.CompileWith(source, blade.CompileOptions{Vars: in.BladeVars(), Defined: in.Complete, Custom: p.bladeCustom()})
		diags = toTemplate(doc, p.analyzeNative(cfg, doc.PHP, shared, true, !in.Complete))
		diags = append(diags, p.analyzeBlade(cfg, source)...)
	} else {
		diags = p.analyzeNative(cfg, source, shared, false, workspace.IsTemplate(uri))
	}
	p.mu.RLock()
	if cached, ok := p.toolResults[uri]; ok {
		diags = append(diags, cached...)
	}
	if cached, ok := p.saveResults[uri]; ok && !withSaveRules {
		diags = append(diags, cached.diags...)
	}
	p.mu.RUnlock()
	if withSaveRules {
		diags = append(diags, p.saveDiagnostics(cfg, uri, source)...)
	}

	// Soft-mode the unknown-* rules until the workspace + composer index has
	// completed at least once. The rules still ran (for testability), but their
	// findings depend on cross-file symbol presence and produce false positives
	// during the warm-up window.
	if p.index != nil && !p.index.Ready() {
		diags = filterOutCodes(diags, rule.UnknownClass, rule.UnknownFunction, rule.UnknownMember, rule.InvalidOverride)
	}

	return p.dropSuppressed(cfg, uri, source, shared, diags)
}

func (p *Provider) analyzeNative(cfg *config.Config, source string, shared *checks.Shared, bladePHP, includedFile bool) []Diagnostic {
	rules := EditRules(p.ruleOptions(bladePHP, includedFile), shared)
	return toDiagnostics(cfg, p.runRules(cfg, rules, parser.ParseFile(source), source))
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

func (p *Provider) runRules(cfg *config.Config, rules []checks.Rule, file *parser.FileNode, source string) []checks.Finding {
	var findings []checks.Finding
	for _, r := range rules {
		if slices.ContainsFunc(r.Codes(), cfg.IsRuleEnabled) {
			findings = append(findings, r.Check(file, source, p.index)...)
		}
	}
	return findings
}

func toDiagnostics(cfg *config.Config, findings []checks.Finding) []Diagnostic {
	diags := make([]Diagnostic, 0, len(findings))
	for _, f := range findings {
		if !cfg.IsRuleEnabled(f.Code) {
			continue
		}
		f.Severity = cfg.RuleSeverity(f.Code, f.Severity)
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

func (p *Provider) analyzeBlade(cfg *config.Config, source string) []Diagnostic {
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
	return toDiagnostics(cfg, findings)
}

// toTemplate moves diagnostics found in a Blade view's virtual PHP to the
// view. Those that start in generated PHP have no place in the view and are
// dropped.
func toTemplate(doc *blade.Document, diags []Diagnostic) []Diagnostic {
	out := diags[:0]
	for _, d := range diags {
		r := d.Range
		sl, sc, el, ec, ok := doc.TemplateRange(r.Start.Line, r.Start.Character, r.End.Line, r.End.Character)
		if !ok {
			continue
		}
		d.Range = textpos.Range{Start: textpos.Position{Line: sl, Character: sc}, End: textpos.Position{Line: el, Character: ec}}
		out = append(out, d)
	}
	return out
}

// AnalyzeOnSave runs heavier checks that need type resolution or model lookups.
// Called on file save only. Results are cached and merged into Analyze() output.
// Findings worked out under settings replaced meanwhile are worked out again.
func (p *Provider) AnalyzeOnSave(uri, source string) {
	for {
		current := p.settings.Load()
		diags := p.saveDiagnostics(current.cfg, uri, source)
		p.mu.Lock()
		stored := p.storeSavedLocked(uri, source, diags, current)
		p.mu.Unlock()
		if stored {
			return
		}
	}
}

func (p *Provider) storeSavedLocked(uri, source string, diags []Diagnostic, under *settings) bool {
	if p.settings.Load() != under {
		return false
	}
	p.saveGen++
	p.saveResults[uri] = savedFindings{source: source, diags: diags, gen: p.saveGen, settings: under}
	return true
}

// saveDiagnostics runs the rules that wait for a save.
func (p *Provider) saveDiagnostics(cfg *config.Config, uri, source string) []Diagnostic {
	var doc *blade.Document
	if workspace.IsBlade(uri) {
		doc = blade.CompileWith(source, blade.CompileOptions{Vars: p.ViewTypes.For(uri).BladeVars(), Custom: p.bladeCustom()})
		source = doc.PHP
	}
	diags := toDiagnostics(cfg, p.runRules(cfg, SaveRules(p.ruleOptions(doc != nil, false)), parser.ParseFile(source), source))
	if doc != nil {
		diags = toTemplate(doc, diags)
	}
	return diags
}

// RunTools executes external analysis tools (PHPStan, Pint) on a file.
// Results are cached and included in subsequent Analyze calls, unless the
// tools' settings changed while they ran.
func (p *Provider) RunTools(uri, filePath string) {
	tools := p.settings.Load()
	var toolDiags []Diagnostic
	if tools.phpstan.available() {
		toolDiags = append(toolDiags, tools.phpstan.analyze(filePath)...)
	}
	if tools.pint.available() {
		toolDiags = append(toolDiags, tools.pint.analyze(filePath)...)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.settings.Load().sameTools(tools) {
		p.toolResults[uri] = toolDiags
	}
}

// ReanalyzeSaved works out the on-save rules' findings again, with the
// current settings, for every saved source whose findings were worked out
// under other settings. A result is replaced only if no newer save stored
// one meanwhile; one worked out under settings replaced meanwhile is worked
// out again.
func (p *Provider) ReanalyzeSaved() {
	for p.reanalyzeStale() {
	}
}

func (p *Provider) reanalyzeStale() bool {
	type saved struct {
		uri, source string
		gen         uint64
	}
	current := p.settings.Load()
	p.mu.RLock()
	var stale []saved
	for uri, found := range p.saveResults {
		if found.settings != current {
			stale = append(stale, saved{uri: uri, source: found.source, gen: found.gen})
		}
	}
	p.mu.RUnlock()
	for _, s := range stale {
		diags := p.saveDiagnostics(current.cfg, s.uri, s.source)
		p.mu.Lock()
		if found, ok := p.saveResults[s.uri]; ok && found.gen == s.gen {
			p.storeSavedLocked(s.uri, s.source, diags, current)
		}
		p.mu.Unlock()
	}
	return len(stale) > 0
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
func filterOutCodes(diags []Diagnostic, codes ...rule.Code) []Diagnostic {
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

func findingToDiagnostic(f checks.Finding) Diagnostic {
	d := Diagnostic{
		Range: textpos.Range{
			Start: textpos.Position{Line: f.StartLine, Character: f.StartCol},
			End:   textpos.Position{Line: f.EndLine, Character: f.EndCol},
		},
		Severity: f.Severity,
		Source:   string(rule.SourceTusk),
		Message:  f.Message,
		Code:     string(f.Code),
	}
	d.Tags = append(d.Tags, f.Tags...)
	return d
}

// NativeFindings returns what every native rule finds in a PHP file's
// source, with nothing filtered by the configuration or suppressed: the raw
// findings introspection shows.
func (p *Provider) NativeFindings(source string) []checks.Finding {
	rules := NativeRules(p.ruleOptions(false, false))
	return checks.CheckFile(parser.ParseFile(source), source, p.index, rules)
}
