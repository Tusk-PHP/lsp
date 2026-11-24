package lsp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/composer/cardhover"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/coverage"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/hover"
	"github.com/Tusk-PHP/lsp/internal/inlayhint"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func (s *Server) handleInitialize(msg *jsonRPCMessage) {
	var params protocol.InitializeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	s.rootPath = symbols.URIToPath(params.RootURI)
	if s.rootPath == "" || s.rootPath == "." {
		s.rootPath = params.RootPath
	}
	s.logger.Printf("Initializing for workspace: %s", s.rootPath)
	cfgPath := filepath.Join(s.rootPath, ".tusk-php.json")
	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil {
		s.logger.Printf("Config error: %v", err)
		s.sendNotification("window/showMessage", map[string]interface{}{
			"type":    protocol.MessageTypeError,
			"message": "Tusk PHP ignored its configuration: " + err.Error(),
		})
	} else {
		s.cfg = cfg
	}
	// Merge client initializationOptions over file-based config
	if opts := params.InitializationOptions; opts != nil {
		s.cfg.MergeClientOptions(opts)
	}
	s.clientSupportsShowDocument = params.Capabilities.Window.ShowDocument.Support
	if !s.clientSupportsShowDocument {
		s.logger.Printf("PHP LSP: client does not advertise window/showDocument support; builtin Cmd+Click → manual will be disabled if enabled in config")
	}
	if s.cfg.PHPManualOpenOnDefinition && !s.clientSupportsShowDocument {
		s.manualFlagWarnedOnce.Do(func() {
			s.sendNotification("window/logMessage", map[string]interface{}{
				"type":    protocol.MessageTypeWarning,
				"message": "PHP LSP: php_manual_open_on_definition is enabled but the client does not advertise window/showDocument support; feature disabled",
			})
		})
	}
	if s.cfg.Framework == "auto" {
		s.framework = config.DetectFramework(s.rootPath)
	} else {
		s.framework = s.cfg.Framework
	}
	s.logger.Printf("Detected framework: %s", s.framework)
	profile := s.resolveBuiltinProfile()
	s.workspace = workspace.New(workspace.Options{
		RootPath:       s.rootPath,
		Framework:      s.framework,
		Config:         s.cfg,
		Logger:         s.logger,
		BuiltinProfile: profile,
	})
	s.index = s.workspace.Index
	s.container = s.workspace.Container
	s.routeIndex = s.workspace.RouteIndex
	s.schemaCache = s.workspace.SchemaCache
	arrayResolver := models.NewFrameworkArrayResolver(s.index, s.rootPath, s.framework)
	viewResolver := frameworklaravel.NewViews(s.rootPath)
	translationResolver := frameworklaravel.NewTranslationResolver(s.rootPath)
	symfonyTranslations := frameworksymfony.NewTranslationResolver(s.rootPath)
	s.completion = completion.NewProvider(s.index, s.container, s.framework)
	// Clients that resolve documentation and detail later get member items
	// without them, which keeps long member lists cheap to send.
	lazyDoc, lazyDetail := false, false
	for _, prop := range params.Capabilities.TextDocument.Completion.CompletionItem.ResolveSupport.Properties {
		switch prop {
		case "documentation":
			lazyDoc = true
		case "detail":
			lazyDetail = true
		}
	}
	s.completion.SetLazyResolve(lazyDoc, lazyDetail)
	if s.cfg != nil {
		s.completion.SetAutoImportAliasStyle(s.cfg.AutoImport.AliasStyle)
	}
	s.completion.SetArrayResolver(arrayResolver)
	if s.routeIndex != nil {
		s.completion.SetLaravelRouteIndex(s.routeIndex)
	}
	s.completion.SetViewResolver(viewResolver)
	s.completion.SetTranslationResolver(translationResolver)
	s.completion.SetSymfonyTranslationResolver(symfonyTranslations)
	s.completion.SetSymfonyCommands(s.workspace.SymfonyCommands)
	s.inlayHint = inlayhint.NewProvider(s.index, s.container)
	s.inlayHint.SetTypedChainResolver(func(expr, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
		return s.completion.ResolveExpressionTypeTyped(expr, source, pos, file)
	})
	s.hover = hover.NewProvider(s.index, s.container, s.framework)
	s.hover.SetArrayResolver(arrayResolver)
	s.hover.SetConfig(s.cfg)
	s.coverage = coverage.NewStore(s.rootPath, s.cfg.CoverageReportPath, s.logger)
	s.hover.SetCoverage(s.coverage)
	s.coverageNotifications = params.InitializationOptions != nil && params.InitializationOptions.CoverageNotifications
	if s.coverageNotifications {
		s.coverage.OnChange(func() { s.sendNotification("tusk/coverageChanged", nil) })
	}
	s.hover.SetTypedChainResolver(func(expr, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
		return s.completion.ResolveExpressionTypeTyped(expr, source, pos, file)
	})
	s.composerHover = cardhover.NewProvider(s.rootPath, profile.PHPVersion)
	s.composerHover.SetEnabled(s.cfg.Composer.Hover.Enable)
	s.composerHover.SetOpenOnDefinition(s.cfg.Composer.OpenOnDefinition)
	if s.cfg.Composer.OpenOnDefinition && !s.clientSupportsShowDocument {
		s.composerOpenFlagWarnedOnce.Do(func() {
			s.sendNotification("window/logMessage", map[string]interface{}{
				"type":    protocol.MessageTypeWarning,
				"message": "PHP LSP: composer.openOnDefinition is enabled but the client does not advertise window/showDocument support; feature disabled",
			})
		})
	}
	s.diag = diagnostics.NewProvider(s.index, s.framework, s.rootPath, s.logger, s.cfg)
	s.diag.TypeResolver = func(expr, source string, line, col int, file *parser.FileNode) string {
		return s.completion.ResolveDiagnosticType(expr, source, protocol.Position{Line: line, Character: col}, file)
	}
	s.diag.BuilderModelResolver = s.completion.ResolveBuilderModel
	s.diag.BuilderMemberChecker = diagnostics.NewIndexMemberChecker(s.index, s.workspace.SchemaIsLive)
	s.diag.BuiltinUnavailableRule = &checks.BuiltinUnavailableRule{
		PHPVersion: profile.PHPVersion,
		Extensions: profile.Extensions,
	}
	s.providers = frameworklaravel.NewProviders(s.rootPath, s.index)
	viewResolver.SetProviders(s.providers)
	s.components = frameworklaravel.NewComponents(s.rootPath, s.index, s.providers, s.getDocumentReader())
	s.bladeFeatures = &blade.Features{Views: viewResolver, Components: s.components, Read: s.getDocumentReader(), Custom: s.providers.Custom}
	s.diag.Blade = s.bladeFeatures
	if s.workspace.ViewData != nil {
		s.viewTypes = &frameworklaravel.ViewTypes{
			Data:  s.workspace.ViewData,
			Index: s.index,
			Read:  s.getDocumentReader(),
			Resolve: func(expr, source string, file *parser.FileNode, line, col int) string {
				return s.completion.ResolveExpressionType(expr, source, protocol.Position{Line: line, Character: col}, file)
			},
		}
		s.diag.ViewTypes = s.viewTypes
	}
	s.analyzer = analyzer.NewAnalyzer(s.index, s.container)
	if opts := params.InitializationOptions; opts != nil {
		s.analyzer.SetClientCommands(opts.ClientCommands)
	}
	s.analyzer.SetChainResolver(s.completion.ResolveExpressionType)
	s.analyzer.SetTypedChainResolver(s.completion.ResolveExpressionTypeTyped)
	if s.routeIndex != nil {
		s.analyzer.SetLaravelRouteIndex(s.routeIndex)
	}
	s.analyzer.SetViewResolver(viewResolver)
	s.analyzer.SetTranslationResolver(translationResolver)
	s.analyzer.SetSymfonyTranslationResolver(symfonyTranslations)
	s.analyzer.SetSymfonyCommands(s.workspace.SymfonyCommands)
	s.sendResponse(msg.ID, protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    1, // Full
				Save:      &protocol.SaveOptions{IncludeText: false},
			},
			CompletionProvider:        &protocol.CompletionOptions{TriggerCharacters: []string{".", ">", ":", "$", "\\", "|", "#", "[", "(", "'", "\"", "@"}, ResolveProvider: true},
			HoverProvider:             true,
			DefinitionProvider:        true,
			TypeHierarchyProvider:     true,
			TypeDefinitionProvider:    true,
			ImplementationProvider:    true,
			ReferencesProvider:        true,
			DocumentSymbolProvider:    true,
			DocumentHighlightProvider: true,
			FoldingRangeProvider:      true,
			WorkspaceSymbolProvider:   true,
			SignatureHelpProvider:     &protocol.SignatureHelpOptions{TriggerCharacters: []string{"(", ","}},
			RenameProvider:            &protocol.RenameOptions{PrepareProvider: true},
			CodeActionProvider:        &protocol.CodeActionOptions{CodeActionKinds: protocol.CodeActionKinds},
			ExecuteCommandProvider:    &protocol.ExecuteCommandOptions{Commands: protocol.Commands},
			InlayHintProvider:         &protocol.InlayHintOptions{ResolveProvider: false},
			SelectionRangeProvider:    true,
			SemanticTokensProvider: &protocol.SemanticTokensOptions{
				Legend: SemanticTokensLegend,
				Full:   true,
			},
			Experimental: &protocol.ExperimentalCapabilities{TuskCoverage: true},
		},
		ServerInfo: protocol.ServerInfo{Name: ServerName, Version: ServerVersion},
	})
}

// resolveBuiltinProfile applies the PHP profile fallback chain:
//  1. composer.json (config.platform.php or require.php)
//  2. .tusk-php.json / initializationOptions phpVersion (when explicitly set)
//  3. locally installed php binary (via phpdetect)
//  4. bundled default
//
// The resolved profile is cached on the Server and surfaced via window/logMessage.
func (s *Server) resolveBuiltinProfile() symbols.BuiltinProfile {
	timeout := time.Duration(s.cfg.PHPDetectTimeoutMs) * time.Millisecond
	profile, source := workspace.ResolveBuiltinProfileWithConfig(s.rootPath, s.cfg.PHPBinary, timeout, s.cfg, func(msg string) {
		s.sendNotification("window/logMessage", map[string]interface{}{
			"type":    protocol.MessageTypeInfo,
			"message": msg,
		})
	})
	s.builtinPHPVersion = profile.PHPVersion
	s.builtinPHPSource = source
	return profile
}

func (s *Server) handleInitialized(msg *jsonRPCMessage) {
	s.index.SetClassLoader(s.workspace.ClassLoader())
	var indexWg sync.WaitGroup
	indexWg.Add(2)
	s.goSafe("indexWorkspace", func() {
		defer indexWg.Done()
		count := s.workspace.IndexWorkspace()
		s.workspace.AnalyzeModelClasses()
		s.sendNotification("window/logMessage", map[string]interface{}{"type": protocol.MessageTypeInfo, "message": fmt.Sprintf("PHP LSP: Indexed %d files (%s framework)", count, s.framework)})
		if limit := s.cfg.MaxIndexFiles; limit > 0 && count >= limit {
			s.sendNotification("window/logMessage", map[string]interface{}{"type": protocol.MessageTypeWarning, "message": fmt.Sprintf("PHP LSP: stopped indexing at maxIndexFiles (%d files), so classes in the rest of the workspace will be reported as unknown. Set maxIndexFiles to 0 to index everything.", limit)})
		}
	})
	s.goSafe("indexComposerDeps", func() {
		defer indexWg.Done()
		vendorCount := s.workspace.IndexComposerDependencies()
		if vendorCount > 0 {
			s.sendNotification("window/logMessage", map[string]interface{}{"type": protocol.MessageTypeInfo, "message": fmt.Sprintf("PHP LSP: Indexed %d vendor files", vendorCount)})
		}
	})
	s.goSafe("postIndexSettle", func() {
		indexWg.Wait()
		s.index.MarkReady()
		s.republishOpenDocuments()
	})
	s.goSafe("container.Analyze", s.workspace.Container.Analyze)
	s.goSafe("coverage.Refresh", s.coverage.Refresh)
	if s.coverageNotifications {
		s.goSafe("coverage.Watch", s.coverage.Watch)
	}
	if s.routeIndex != nil {
		s.goSafe("laravelRoutes.ScanWorkspace", func() {
			_ = s.workspace.ScanRoutes()
		})
	}
	// Run model analysis after both workspace and vendor indexing complete
	s.goSafe("analyzeModels", func() {
		indexWg.Wait()
		s.workspace.AnalyzeModels()
	})
	s.goSafe("symfonyCommands", func() {
		indexWg.Wait()
		s.workspace.ScanSymfonyCommands()
	})
}
