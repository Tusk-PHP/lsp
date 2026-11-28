package lsp

import (
	"fmt"
	"sync"
	"time"

	"github.com/Tusk-PHP/lsp/internal/composer/cardhover"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/coverage"
	"github.com/Tusk-PHP/lsp/internal/document"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/session"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func (s *Server) handleInitialize(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[initializeParams](msg)
	if !ok {
		return
	}
	s.rootPath = s.resolveRoot(params)
	s.logger.Printf("Initializing for workspace: %s", s.rootPath)
	s.initOptions = params.InitializationOptions.settings()
	s.cfg = s.loadConfig()
	s.encoding = document.Negotiate(params.Capabilities.General.PositionEncodings)
	s.clientSupportsShowDocument = params.Capabilities.Window.ShowDocument.Support
	s.clientWatchesFiles = params.Capabilities.Workspace.DidChangeWatchedFiles.DynamicRegistration
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
	profile := s.resolveBuiltinProfile(s.cfg)
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
	s.session = session.New(s.workspace, session.Options{Read: s.getDocumentReader(), Logger: s.logger})
	s.analysis = s.session
	s.completion = s.session.Completion()
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
	s.inlayHint = s.session.InlayHints()
	s.hover = s.session.Hover()
	s.coverage = coverage.NewStore(s.rootPath, s.cfg.CoverageReportPath, s.logger)
	s.hover.SetCoverage(s.coverage)
	s.coverageNotifications = params.InitializationOptions != nil && params.InitializationOptions.CoverageNotifications
	if s.coverageNotifications {
		s.coverage.OnChange(func() { s.sendNotification("tusk/coverageChanged", nil) })
	}
	s.composerHover = cardhover.NewProvider(s.rootPath, func() string { return s.workspace.Profile().PHPVersion })
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
	s.analyzer = s.session.Analyzer()
	if opts := params.InitializationOptions; opts != nil {
		s.analyzer.SetClientCommands(opts.ClientCommands)
	}
	s.sendResponse(msg.ID, protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			PositionEncoding: s.encoding.String(),
			TextDocumentSync: protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    1, // Full
				Save:      &protocol.SaveOptions{IncludeText: false},
			},
			CompletionProvider:        &protocol.CompletionOptions{TriggerCharacters: []string{".", ">", ":", "$", "\\", "|", "#", "[", "(", "'", "\"", "@"}, ResolveProvider: true},
			HoverProvider:             true,
			DefinitionProvider:        true,
			TypeHierarchyProvider:     true,
			DocumentLinkProvider:      &protocol.DocumentLinkOptions{},
			DeclarationProvider:       true,
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

func (s *Server) resolveBuiltinProfile(cfg *config.Config) symbols.BuiltinProfile {
	timeout := time.Duration(cfg.PHPDetectTimeoutMs) * time.Millisecond
	profile, _ := workspace.ResolveBuiltinProfileWithConfig(s.rootPath, cfg.PHPBinary, timeout, cfg, s.logInfo)
	return profile
}

func (s *Server) logInfo(msg string) {
	s.sendNotification("window/logMessage", map[string]interface{}{
		"type":    protocol.MessageTypeInfo,
		"message": msg,
	})
}

func (s *Server) handleInitialized(msg *jsonRPCMessage) {
	s.index.SetClassLoader(s.workspace.ClassLoader())
	var indexWg sync.WaitGroup
	indexWg.Add(2)
	s.goSafe("indexWorkspace", func() {
		defer indexWg.Done()
		indexed := s.workspace.IndexWorkspace()
		s.workspace.AnalyzeModelClasses()
		s.sendNotification("window/logMessage", map[string]interface{}{"type": protocol.MessageTypeInfo, "message": fmt.Sprintf("PHP LSP: Indexed %d files (%s framework)", indexed.Count, s.framework)})
		if indexed.Truncated {
			s.sendNotification("window/logMessage", map[string]interface{}{"type": protocol.MessageTypeWarning, "message": "PHP LSP: " + workspace.MaxIndexFilesWarning(s.cfg.MaxIndexFiles)})
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
		close(s.indexed)
		s.logInfo("PHP LSP: Index ready")
		s.republishOpenDocuments()
	})
	s.watchFiles()
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
