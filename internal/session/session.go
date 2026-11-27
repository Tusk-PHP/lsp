// Package session builds and connects the analysis for a project once, for
// every transport: the language server, the MCP server and the CLI get their
// diagnostics and editor features through a Session, and none of them
// builds rules or wires resolvers between those providers itself. A transport
// adds only what is its own, such as client capabilities.
package session

import (
	"context"
	"io"
	"log"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/hover"
	"github.com/Tusk-PHP/lsp/internal/inlayhint"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// Options say how a session reads files and where it logs.
type Options struct {
	// Read returns a file's source by URI: the editor's unsaved buffer when
	// it has one, else the file on disk. Blade views read the components
	// they use through it. Nil reads from disk.
	Read func(uri string) string
	// Logger receives the external tools' output (PHPStan, Pint). Nil
	// discards it.
	Logger *log.Logger
}

func (o Options) logger() *log.Logger {
	if o.Logger == nil {
		return log.New(io.Discard, "", 0)
	}
	return o.Logger
}

// Session is the analysis wired over one workspace.
type Session struct {
	ws         *workspace.Bootstrapped
	diag       *diagnostics.Provider
	views      *frameworklaravel.Views
	providers  *frameworklaravel.Providers
	components *frameworklaravel.Components
	blade      *blade.Features
	viewTypes  *frameworklaravel.ViewTypes

	translations        *frameworklaravel.TranslationResolver
	symfonyTranslations *frameworksymfony.TranslationResolver
	completion          *completion.Provider
	hover               *hover.Provider
	inlayHint           *inlayhint.Provider
	analyzer            *analyzer.Analyzer
}

// New wires the analysis over ws, which may still be indexing: findings
// that depend on the whole index wait until it is ready.
func New(ws *workspace.Bootstrapped, opts Options) *Session {
	read := opts.Read
	if read == nil {
		read = readFile
	}
	logger := opts.logger()

	s := &Session{ws: ws}
	engine := diagnostics.TypeEngine(ws.Index, ws.Container)

	s.diag = diagnostics.NewProvider(ws.Index, ws.Framework, ws.RootPath, logger, ws.Config)
	s.diag.ResolveTypes(ws.Container)
	s.diag.BuilderMemberChecker = diagnostics.NewIndexMemberChecker(ws.Index, ws.SchemaIsLive)
	s.diag.Profile = ws.Profile

	s.views = frameworklaravel.NewViews(ws.RootPath)
	s.providers = frameworklaravel.NewProviders(ws.RootPath, ws.Index)
	s.views.SetProviders(s.providers)
	s.components = frameworklaravel.NewComponents(ws.RootPath, ws.Index, s.providers, read)
	s.blade = &blade.Features{Views: s.views, Components: s.components, Read: read, Custom: s.providers.Custom}
	s.diag.Blade = s.blade
	if ws.ViewData != nil {
		s.viewTypes = &frameworklaravel.ViewTypes{
			Data:  ws.ViewData,
			Index: ws.Index,
			Read:  read,
			Resolve: func(expr, source string, file *parser.FileNode, line, col int) string {
				return engine.Resolver().ExpressionClass(expr, source, textpos.Position{Line: line, Character: col}, file)
			},
		}
		s.diag.ViewTypes = s.viewTypes
	}

	s.translations = frameworklaravel.NewTranslationResolver(ws.RootPath)
	s.symfonyTranslations = frameworksymfony.NewTranslationResolver(ws.RootPath)
	arrays := models.NewFrameworkArrayResolver(ws.Index, ws.RootPath, ws.Framework)

	s.completion = completion.NewProvider(ws.Index, ws.Container, ws.Framework)
	s.completion.SetAutoImportAliasStyle(ws.Config.AutoImport.AliasStyle)
	s.completion.SetArrayResolver(arrays)
	if ws.RouteIndex != nil {
		s.completion.SetLaravelRouteIndex(ws.RouteIndex)
	}
	s.completion.SetViewResolver(s.views)
	s.completion.SetTranslationResolver(s.translations)
	s.completion.SetSymfonyTranslationResolver(s.symfonyTranslations)
	s.completion.SetSymfonyCommands(ws.SymfonyCommands)

	s.hover = hover.NewProvider(ws.Index, ws.Container, ws.Framework)
	s.hover.SetArrayResolver(arrays)
	s.hover.SetConfig(ws.Config)
	s.hover.SetTypedChainResolver(s.typedExpression)

	s.inlayHint = inlayhint.NewProvider(ws.Index, ws.Container)
	s.inlayHint.SetTypedChainResolver(s.typedExpression)

	s.analyzer = analyzer.NewAnalyzer(ws.Index, ws.Container)
	s.analyzer.SetChainResolver(s.expression)
	s.analyzer.SetTypedChainResolver(s.typedExpression)
	if ws.RouteIndex != nil {
		s.analyzer.SetLaravelRouteIndex(ws.RouteIndex)
	}
	s.analyzer.SetViewResolver(s.views)
	s.analyzer.SetTranslationResolver(s.translations)
	s.analyzer.SetSymfonyTranslationResolver(s.symfonyTranslations)
	s.analyzer.SetSymfonyCommands(ws.SymfonyCommands)
	return s
}

// Open loads the project at root (its .tusk-php.json, framework and PHP
// profile), indexes it completely, and wires the analysis over it. Use it
// for one-shot runs; a long-lived server indexes in the background and
// calls New.
func Open(ctx context.Context, root string, opts Options) (*Session, error) {
	project, err := workspace.LoadProject(root, nil)
	if err != nil {
		return nil, err
	}
	ws, err := workspace.Build(ctx, workspace.Options{
		RootPath:       root,
		Framework:      project.Framework,
		Config:         project.Config,
		Logger:         opts.logger(),
		BuiltinProfile: project.Profile,
	})
	if err != nil {
		return nil, err
	}
	return New(ws, opts), nil
}

// Config is the project's configuration.
func (s *Session) Config() *config.Config { return s.ws.Config }

// Analyze returns a file's diagnostics while it is being edited: the fast
// rules, plus the latest on-save and external-tool results for it.
func (s *Session) Analyze(uri, source string) []diagnostics.Diagnostic {
	return s.diag.Analyze(uri, source)
}

// AnalyzeAll returns every diagnostic for a file as it is on disk, on-save
// rules included, computed now: for one-shot runs such as `tusk-php check`
// and the MCP diagnostics tool.
func (s *Session) AnalyzeAll(uri, source string) []diagnostics.Diagnostic {
	return s.diag.AnalyzeAll(uri, source)
}

// Saved runs the on-save rules over a saved file and keeps their findings
// for Analyze to report.
func (s *Session) Saved(uri, source string) {
	s.diag.AnalyzeOnSave(uri, source)
}

// RunTools runs the configured external tools (PHPStan, Pint) over a saved
// file in the background and keeps their findings for Analyze to report.
func (s *Session) RunTools(uri, filePath string) {
	s.diag.RunTools(uri, filePath)
}

// Closed forgets what was kept for a file.
func (s *Session) Closed(uri string) {
	s.diag.ClearCache(uri)
}

// NativeFindings returns what every native rule finds in a PHP file's
// source, with nothing filtered or suppressed: the raw findings introspection
// shows.
func (s *Session) NativeFindings(source string) []checks.Finding {
	return s.diag.NativeFindings(source)
}

// SourceFiles lists the project's own files that are checked: its indexed
// PHP files and Blade views, as paths, vendor/ included.
func (s *Session) SourceFiles() []string {
	var paths []string
	for _, uri := range s.ws.Index.GetAllFileURIs() {
		if path := symbols.URIToPath(uri); path != "" && workspace.IsPHPSource(path) {
			paths = append(paths, path)
		}
	}
	// Blade views declare no symbols, but they are checked like the rest.
	for _, uri := range s.ws.Index.TemplateFileURIs() {
		if path := symbols.URIToPath(uri); path != "" && workspace.IsBlade(path) {
			paths = append(paths, path)
		}
	}
	return paths
}

// Completion answers completion requests. A transport sets only its
// client's own preferences on it, such as lazy resolution.
func (s *Session) Completion() *completion.Provider { return s.completion }

// Hover answers hover requests. A transport adds only what it owns, such as
// a coverage store it keeps current.
func (s *Session) Hover() *hover.Provider { return s.hover }

// InlayHints answers inlay hint requests.
func (s *Session) InlayHints() *inlayhint.Provider { return s.inlayHint }

// Analyzer answers definition, references, symbols, rename and signature
// help. A transport adds only what it owns, such as the commands its client
// can run.
func (s *Session) Analyzer() *analyzer.Analyzer { return s.analyzer }

// expression gives the class an expression evaluates to, as completion
// resolves it, for the analyzer, which types method chains the same way.
func (s *Session) expression(expr, source string, pos textpos.Position, file *parser.FileNode) string {
	return s.completion.ResolveExpressionType(expr, source, pos, file)
}

// typedExpression gives an expression's type with its generic arguments, as
// completion resolves it, for hover, inlay hints and the analyzer.
func (s *Session) typedExpression(expr, source string, pos textpos.Position, file *parser.FileNode) resolve.ResolvedType {
	return s.completion.ResolveExpressionTypeTyped(expr, source, pos, file)
}

// Views resolves Laravel view names, with the project's registered
// namespaces.
func (s *Session) Views() *frameworklaravel.Views { return s.views }

// BladeProviders are what the project's service providers register with
// Blade.
func (s *Session) BladeProviders() *frameworklaravel.Providers { return s.providers }

// Components are the project's Blade components; invalidate them when a
// component class or view changes.
func (s *Session) Components() *frameworklaravel.Components { return s.components }

// Blade serves completion, hover and definition inside Blade views.
func (s *Session) Blade() *blade.Features { return s.blade }

// ViewTypes types the variables Blade views receive; nil outside a Laravel
// app.
func (s *Session) ViewTypes() *frameworklaravel.ViewTypes { return s.viewTypes }
