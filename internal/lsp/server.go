package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/composer/cardhover"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/hover"
	"github.com/Tusk-PHP/lsp/internal/inlayhint"
	"github.com/Tusk-PHP/lsp/internal/introspect"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

const ServerName = "tusk-php"

// ServerVersion is overridden at startup from the binary's stamped version.
var ServerVersion = "0.5.0"

// largeDocThreshold is the line-count above which document indexing on
// didOpen/didChange is offloaded to a background goroutine via goSafe so
// that the JSON-RPC message loop is not stalled by expensive parses.
// Documents at or below this threshold are indexed synchronously, which
// keeps ordinary files (and all test fixtures) deterministic.
const largeDocThreshold = 5000

type Server struct {
	cfg           *config.Config
	workspace     *workspace.Bootstrapped
	index         *symbols.Index
	container     *container.ContainerAnalyzer
	completion    *completion.Provider
	hover         *hover.Provider
	composerHover *cardhover.Provider
	inlayHint     *inlayhint.Provider
	diag          *diagnostics.Provider
	analyzer      *analyzer.Analyzer
	routeIndex    *frameworklaravel.RouteIndex
	schemaCache   *models.SchemaCache
	docMu         sync.RWMutex
	documents     map[string]string
	rootPath      string
	framework     string
	reader        *bufio.Reader
	writer        io.Writer
	logger        *log.Logger
	shutdown      bool
	strict        bool      // when true, recovered panics are re-raised after logging
	exitFunc      func(int) // called by the "exit" notification handler; defaults to os.Exit

	builtinPHPVersion string
	builtinPHPSource  string // "composer" | "local" | "fallback"

	clientSupportsShowDocument bool

	// requestIDCounter generates unique IDs for server-to-client requests.
	// Accessed via atomic operations.
	requestIDCounter int64

	// manualFlagWarnedOnce ensures the "php_manual_open_on_definition is set but
	// client does not support window/showDocument" warning is emitted at most once
	// per session (logged during initialize).
	manualFlagWarnedOnce sync.Once

	// composerOpenFlagWarnedOnce mirrors manualFlagWarnedOnce for the
	// composer.openOnDefinition flag.
	composerOpenFlagWarnedOnce sync.Once
}

func NewServer(reader io.Reader, writer io.Writer, logger *log.Logger) *Server {
	return &Server{cfg: config.DefaultConfig(), index: symbols.NewIndex(), schemaCache: models.NewSchemaCache(), documents: make(map[string]string), reader: bufio.NewReader(reader), writer: writer, logger: logger, exitFunc: os.Exit}
}

// SetStrict enables or disables strict panic mode.
//
// In strict mode (strict=true) any panic recovered by recoverPanic — including
// panics that originate inside goSafe background goroutines — is re-raised
// after logging. This means a single recovered panic in any goroutine will
// terminate the process, making it suitable for conformance test harnesses that
// must treat "should-not-happen" panics as fatal failures rather than silently
// swallowing them.
//
// In the default (non-strict) mode all panics are caught, logged, and the
// server continues running; individual requests may silently return no result.
func (s *Server) SetStrict(strict bool) {
	s.strict = strict
}

func (s *Server) Run() error {
	s.logger.Println("PHP LSP server starting...")
	for {
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			s.logger.Printf("Read error: %v", err)
			continue
		}
		func() {
			defer s.recoverPanic("handleMessage")
			s.handleMessage(msg)
		}()
	}
}

// recoverPanic is intended to be called via defer. It catches any in-flight
// panic, logs it with a stack trace, and — when strict mode is active — re-
// raises it so the process terminates. This re-raise behaviour also applies to
// panics originating inside goSafe background goroutines; in strict mode even
// a background goroutine panic is fatal.
func (s *Server) recoverPanic(context string) {
	if r := recover(); r != nil {
		s.logger.Printf("Panic in %s: %v\n%s", context, r, debug.Stack())
		if s.strict {
			panic(r)
		}
	}
}

func (s *Server) goSafe(context string, fn func()) {
	go func() {
		defer s.recoverPanic(context)
		fn()
	}()
}

type jsonRPCMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) readMessage() (*jsonRPCMessage, error) {
	var contentLength int
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			lengthStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(lengthStr)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %v", err)
			}
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, body); err != nil {
		return nil, err
	}
	var msg jsonRPCMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	s.logger.Printf("← %s", msg.Method)
	return &msg, nil
}

func (s *Server) sendResponse(id *json.RawMessage, result interface{}) {
	if id == nil {
		return
	}
	s.writeMessage(struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      interface{} `json:"id"`
		Result  interface{} `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) sendError(id *json.RawMessage, code int, message string) {
	if id == nil {
		return
	}
	s.writeMessage(jsonRPCResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func (s *Server) sendNotification(method string, params interface{}) {
	s.writeMessage(struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{JSONRPC: "2.0", Method: method, Params: params})
}

// sendServerRequest sends a server-to-client JSON-RPC request. The response
// is intentionally ignored ("fire-and-forget"). Use this for one-way side
// effects such as window/showDocument where the server has no business
// logic dependent on the client's reply.
//
// Each call generates a unique numeric ID. The client's response (if any)
// arrives back through the normal message loop and is silently dropped.
func (s *Server) sendServerRequest(method string, params interface{}) {
	id := atomic.AddInt64(&s.requestIDCounter, 1)
	s.writeMessage(struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      int64       `json:"id"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{JSONRPC: "2.0", ID: id, Method: method, Params: params})
}

func (s *Server) writeMessage(msg interface{}) {
	body, err := json.Marshal(msg)
	if err != nil {
		s.logger.Printf("Marshal error: %v", err)
		return
	}
	fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n%s", len(body), body)
}

func (s *Server) handleMessage(msg *jsonRPCMessage) {
	// Responses to server-initiated requests (e.g. window/showDocument) are
	// fire-and-forget; we drop them silently.
	if msg.Method == "" && msg.ID != nil {
		return
	}
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		s.handleInitialized(msg)
	case "shutdown":
		s.shutdown = true
		s.sendResponse(msg.ID, nil)
	case "exit":
		s.exitFunc(0)
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/didSave":
		s.handleDidSave(msg)
	case "textDocument/completion":
		s.handleCompletion(msg)
	case "textDocument/hover":
		s.handleHover(msg)
	case "textDocument/definition":
		s.handleDefinition(msg)
	case "textDocument/typeDefinition":
		s.handleTypeDefinition(msg)
	case "textDocument/implementation":
		s.handleImplementation(msg)
	case "textDocument/references":
		s.handleReferences(msg)
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(msg)
	case "textDocument/foldingRange":
		s.handleFoldingRange(msg)
	case "textDocument/documentHighlight":
		s.handleDocumentHighlight(msg)
	case "textDocument/signatureHelp":
		s.handleSignatureHelp(msg)
	case "textDocument/prepareRename":
		s.handlePrepareRename(msg)
	case "textDocument/rename":
		s.handleRename(msg)
	case "textDocument/codeAction":
		s.handleCodeAction(msg)
	case "textDocument/inlayHint":
		s.handleInlayHint(msg)
	case "textDocument/selectionRange":
		s.handleSelectionRange(msg)
	case "textDocument/semanticTokens/full":
		s.handleSemanticTokensFull(msg)
	case "workspace/symbol":
		s.handleWorkspaceSymbol(msg)
	case "workspace/executeCommand":
		s.handleExecuteCommand(msg)
	default:
		if msg.ID != nil {
			s.sendError(msg.ID, -32601, fmt.Sprintf("Method not found: %s", msg.Method))
		}
	}
}

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
	if cfg, err := config.LoadFromFile(cfgPath); err == nil {
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
	s.completion = completion.NewProvider(s.index, s.container, s.framework)
	s.completion.SetArrayResolver(arrayResolver)
	if s.routeIndex != nil {
		s.completion.SetLaravelRouteIndex(s.routeIndex)
	}
	s.completion.SetViewResolver(viewResolver)
	s.completion.SetTranslationResolver(translationResolver)
	s.inlayHint = inlayhint.NewProvider(s.index, s.container)
	s.inlayHint.SetTypedChainResolver(func(expr, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
		return s.completion.ResolveExpressionTypeTyped(expr, source, pos, file)
	})
	s.hover = hover.NewProvider(s.index, s.container, s.framework)
	s.hover.SetArrayResolver(arrayResolver)
	s.hover.SetConfig(s.cfg)
	s.hover.GenericExprResolver = func(expr, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
		return s.completion.ResolveExpressionTypeTyped(expr, source, pos, file)
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
	s.diag.TypeResolver = func(expr, source string, line int, file *parser.FileNode) string {
		return s.completion.ResolveExpressionType(expr, source, protocol.Position{Line: line}, file)
	}
	s.diag.BuilderMemberChecker = diagnostics.NewIndexMemberChecker(s.index)
	s.diag.BuiltinUnavailableRule = &checks.BuiltinUnavailableRule{
		PHPVersion: profile.PHPVersion,
		Extensions: profile.Extensions,
	}
	s.analyzer = analyzer.NewAnalyzer(s.index, s.container)
	s.analyzer.SetChainResolver(s.completion.ResolveExpressionType)
	s.analyzer.SetTypedChainResolver(s.completion.ResolveExpressionTypeTyped)
	if s.routeIndex != nil {
		s.analyzer.SetLaravelRouteIndex(s.routeIndex)
	}
	s.analyzer.SetViewResolver(viewResolver)
	s.analyzer.SetTranslationResolver(translationResolver)
	s.sendResponse(msg.ID, protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    1, // Full
				Save:      &protocol.SaveOptions{IncludeText: false},
			},
			CompletionProvider:        &protocol.CompletionOptions{TriggerCharacters: []string{".", ">", ":", "$", "\\", "|", "#", "[", "(", "'", "\""}, ResolveProvider: false},
			HoverProvider:             true,
			DefinitionProvider:        true,
			TypeDefinitionProvider:    true,
			ImplementationProvider:    true,
			ReferencesProvider:        true,
			DocumentSymbolProvider:    true,
			DocumentHighlightProvider: true,
			FoldingRangeProvider:      true,
			WorkspaceSymbolProvider:   true,
			SignatureHelpProvider:     &protocol.SignatureHelpOptions{TriggerCharacters: []string{"(", ","}},
			RenameProvider:            &protocol.RenameOptions{PrepareProvider: true},
			CodeActionProvider: &protocol.CodeActionOptions{CodeActionKinds: []string{
				"quickfix",
				"refactor",
				"refactor.extract",
				"refactor.move",
				"source",
				"source.organizeImports",
			}},
			ExecuteCommandProvider: &protocol.ExecuteCommandOptions{Commands: []string{
					"tuskPhpLsp.namespaceForPath",
					"tuskPhpLsp.copyNamespace",
					"tuskPhpLsp.moveToNamespace",
					"tuskPhpLsp.debugDocument",
				}},
			InlayHintProvider:      &protocol.InlayHintOptions{ResolveProvider: false},
			SelectionRangeProvider: true,
			SemanticTokensProvider: &protocol.SemanticTokensOptions{
				Legend: SemanticTokensLegend,
				Full:   true,
			},
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
	var indexWg sync.WaitGroup
	indexWg.Add(2)
	s.goSafe("indexWorkspace", func() {
		defer indexWg.Done()
		count := s.workspace.IndexWorkspace()
		s.sendNotification("window/logMessage", map[string]interface{}{"type": protocol.MessageTypeInfo, "message": fmt.Sprintf("PHP LSP: Indexed %d files (%s framework)", count, s.framework)})
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
}

func (s *Server) handleDidOpen(msg *jsonRPCMessage) {
	var params protocol.DidOpenTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil {
		return
	}
	uri := params.TextDocument.URI
	text := params.TextDocument.Text
	s.docMu.Lock()
	s.documents[uri] = text
	s.docMu.Unlock()
	if !isPHPDocument(uri) {
		// Non-PHP documents (composer.json, tsconfig.json, …) are only
		// stored so providers that read source — e.g. composer-hover —
		// can serve them. We never PHP-parse or diagnose them.
		return
	}
	s.indexDocumentMaybeAsync(uri, text)
	if s.cfg.DiagnosticsEnabled {
		s.publishDiagnostics(uri, text)
	}
}

func (s *Server) handleDidChange(msg *jsonRPCMessage) {
	var params protocol.DidChangeTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil {
		return
	}
	if len(params.ContentChanges) > 0 {
		source := params.ContentChanges[len(params.ContentChanges)-1].Text
		uri := params.TextDocument.URI
		s.docMu.Lock()
		s.documents[uri] = source
		s.docMu.Unlock()
		if !isPHPDocument(uri) {
			return
		}
		s.indexDocumentMaybeAsync(uri, source)
		if s.cfg.DiagnosticsEnabled {
			s.publishDiagnostics(uri, source)
		}
	}
}

// isPHPDocument reports whether the URI points at a PHP source file.
// Used to gate PHP-only paths (indexing, diagnostics) so non-PHP
// documents — composer.json, tsconfig.json, etc. — can be opened
// without polluting the symbol index or generating diagnostics.
func isPHPDocument(uri string) bool {
	return strings.HasSuffix(strings.ToLower(uri), ".php")
}

// indexDocumentMaybeAsync indexes source synchronously for small documents
// (line count <= largeDocThreshold) so that ordinary editor interactions
// remain deterministic. For large documents the indexing is offloaded to a
// background goroutine via goSafe so the JSON-RPC message loop is not stalled.
func (s *Server) indexDocumentMaybeAsync(uri, source string) {
	if strings.Count(source, "\n") > largeDocThreshold {
		s.goSafe("indexFileByURI:"+uri, func() {
			s.indexFileByURI(uri, source)
		})
	} else {
		s.indexFileByURI(uri, source)
	}
}

func (s *Server) handleDidClose(msg *jsonRPCMessage) {
	var params protocol.DidCloseTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil {
		return
	}
	s.docMu.Lock()
	delete(s.documents, params.TextDocument.URI)
	s.docMu.Unlock()
	s.diag.ClearCache(params.TextDocument.URI)
}

func (s *Server) handleDidSave(msg *jsonRPCMessage) {
	var params protocol.DidSaveTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil {
		return
	}
	uri := params.TextDocument.URI
	if params.Text != nil {
		s.docMu.Lock()
		s.documents[uri] = *params.Text
		s.docMu.Unlock()
		if isPHPDocument(uri) {
			s.indexFileByURI(uri, *params.Text)
		}
	}
	if !isPHPDocument(uri) {
		return
	}
	s.goSafe("container.Analyze", s.container.Analyze)
	if s.cfg.DiagnosticsEnabled {
		s.goSafe("diagnostics.RunTools", func() {
			filePath := symbols.URIToPath(uri)
			s.diag.RunTools(uri, filePath)
			source := s.getDocument(uri)
			s.diag.AnalyzeOnSave(uri, source)
			s.publishDiagnostics(uri, source)
		})
	}
}

func (s *Server) handleCompletion(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.completion.GetCompletions(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleHover(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	uri := params.TextDocument.URI
	source := s.getDocument(uri)
	if cardhover.IsComposerJSON(uri) {
		s.sendResponse(msg.ID, s.composerHover.Hover(uri, source, params.Position))
		return
	}
	s.sendResponse(msg.ID, s.hover.GetHover(uri, source, params.Position))
}

func (s *Server) handleDefinition(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	uri := params.TextDocument.URI
	source := s.getDocument(uri)

	if cardhover.IsComposerJSON(uri) {
		result := s.composerHover.Definition(uri, source, params.Position)
		if result.ExternalURL != "" && s.clientSupportsShowDocument {
			s.sendServerRequest("window/showDocument", protocol.ShowDocumentParams{
				URI:       result.ExternalURL,
				External:  true,
				TakeFocus: true,
			})
		}
		s.sendResponse(msg.ID, nil)
		return
	}

	openManual := s.cfg.PHPManualOpenOnDefinition && s.clientSupportsShowDocument
	result := s.analyzer.FindDefinitionWithManual(uri, source, params.Position, openManual, s.cfg.PHPManualLocale)

	if result.ExternalManualURL != "" {
		s.sendServerRequest("window/showDocument", protocol.ShowDocumentParams{
			URI:       result.ExternalManualURL,
			External:  true,
			TakeFocus: true,
		})
		s.sendResponse(msg.ID, nil)
		return
	}

	s.sendResponse(msg.ID, result.Location)
}

func (s *Server) handleTypeDefinition(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.FindTypeDefinition(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleImplementation(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.FindImplementation(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleReferences(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.FindReferences(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleDocumentSymbol(msg *jsonRPCMessage) {
	var params struct {
		TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	}
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetDocumentSymbols(params.TextDocument.URI, source))
}

func (s *Server) handleFoldingRange(msg *jsonRPCMessage) {
	var params protocol.FoldingRangeParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetFoldingRanges(params.TextDocument.URI, source))
}

func (s *Server) handleDocumentHighlight(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetDocumentHighlights(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleWorkspaceSymbol(msg *jsonRPCMessage) {
	var params protocol.WorkspaceSymbolParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	s.sendResponse(msg.ID, s.analyzer.GetWorkspaceSymbols(params.Query))
}

func (s *Server) handleSignatureHelp(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetSignatureHelp(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handlePrepareRename(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	result := s.analyzer.PrepareRename(params.TextDocument.URI, source, params.Position)
	s.sendResponse(msg.ID, result)
}

func (s *Server) handleRename(msg *jsonRPCMessage) {
	var params protocol.RenameParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	result := s.analyzer.Rename(params.TextDocument.URI, source, params.Position, params.NewName, s.getDocumentReader())
	s.sendResponse(msg.ID, result)
}

func (s *Server) handleCodeAction(msg *jsonRPCMessage) {
	var params protocol.CodeActionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	result := s.analyzer.GetCodeActions(params.TextDocument.URI, source, params)
	if len(params.Context.Only) > 0 {
		filtered := result[:0]
		for _, action := range result {
			if matchesCodeActionKind(action.Kind, params.Context.Only) {
				filtered = append(filtered, action)
			}
		}
		result = filtered
	}
	s.sendResponse(msg.ID, result)
}

func matchesCodeActionKind(kind string, only []string) bool {
	if kind == "" {
		return false
	}
	for _, filter := range only {
		if kind == filter || strings.HasPrefix(kind, filter+".") {
			return true
		}
	}
	return false
}

func (s *Server) handleInlayHint(msg *jsonRPCMessage) {
	var params protocol.InlayHintParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	hints := s.inlayHint.GetInlayHints(params.TextDocument.URI, source, &s.cfg.InlayHints)
	// Filter to the requested line range.
	filtered := hints[:0]
	for _, h := range hints {
		if h.Position.Line >= params.Range.Start.Line && h.Position.Line <= params.Range.End.Line {
			filtered = append(filtered, h)
		}
	}
	s.sendResponse(msg.ID, filtered)
}

func (s *Server) handleExecuteCommand(msg *jsonRPCMessage) {
	var params protocol.ExecuteCommandParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	switch params.Command {
	case "tuskPhpLsp.copyNamespace":
		if len(params.Arguments) > 0 {
			var uri string
			if json.Unmarshal(params.Arguments[0], &uri) == nil {
				source := s.getDocument(uri)
				ns := s.analyzer.GetFileNamespace(uri, source)
				s.sendResponse(msg.ID, ns)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	case "tuskPhpLsp.namespaceForPath":
		// Returns the expected namespace for a file path based on PSR-4 autoload
		if len(params.Arguments) > 0 {
			var uri string
			if json.Unmarshal(params.Arguments[0], &uri) == nil {
				filePath := symbols.URIToPath(uri)
				autoload := composer.GetAutoloadPaths(s.rootPath)
				ns := composer.PathToNamespace(filePath, autoload)
				s.sendResponse(msg.ID, ns)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	case "tuskPhpLsp.moveToNamespace":
		// Arguments: [uri, targetNamespace]
		if len(params.Arguments) >= 2 {
			var uri, targetNS string
			if json.Unmarshal(params.Arguments[0], &uri) == nil && json.Unmarshal(params.Arguments[1], &targetNS) == nil {
				source := s.getDocument(uri)
				autoload := composer.GetAutoloadPaths(s.rootPath)
				edit := s.analyzer.MoveToNamespace(uri, source, targetNS, autoload, s.getDocumentReader())
				s.sendResponse(msg.ID, edit)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	case "tuskPhpLsp.debugDocument":
		if len(params.Arguments) > 0 {
			var uri string
			if json.Unmarshal(params.Arguments[0], &uri) == nil {
				source := s.getDocument(uri)

				// Determine buffer state: "live-buffer" when the document is
				// open in the editor (present in s.documents), otherwise "disk".
				s.docMu.RLock()
				_, inBuffer := s.documents[uri]
				s.docMu.RUnlock()
				bufferState := "disk"
				if inBuffer {
					bufferState = "live-buffer"
				}

				// Run native checks synchronously for a self-consistent dump.
				file := parser.ParseFile(source)
				var typeRes func(string, string, int, *parser.FileNode) string
				var builtinRule *checks.BuiltinUnavailableRule
				if s.diag != nil {
					typeRes = s.diag.TypeResolver
					builtinRule = s.diag.BuiltinUnavailableRule
				}
				rules := diagnostics.NativeRules(diagnostics.NativeRulesOptions{
					TypeResolver:           typeRes,
					BuiltinUnavailableRule: builtinRule,
				})
				findings := checks.CheckFile(file, source, s.index, rules)

				v := introspect.ParseVerbosity(s.cfg.Introspection.Verbosity)

				var profile string
				if s.workspace != nil {
					bp := s.workspace.BuiltinProfile
					profile = introspect.FormatProfile(bp.PHPVersion, bp.Extensions)
				}

				in := introspect.Input{
					URI:           uri,
					Source:        source,
					Index:         s.index,
					Container:     s.container,
					Framework:     s.framework,
					ServerVersion: ServerVersion,
					Profile:       profile,
					BufferState:   bufferState,
					Diagnostics:   findings,
					Verbosity:     v,
				}
				text := introspect.Render(introspect.Document(in), v)

				s.sendResponse(msg.ID, text)

				// Zed display path: emit the dump to the LSP log panel and
				// show a toast pointing the user there.
				s.sendNotification("window/logMessage", map[string]interface{}{
					"type":    protocol.MessageTypeInfo,
					"message": text,
				})
				s.sendNotification("window/showMessage", map[string]interface{}{
					"type":    protocol.MessageTypeInfo,
					"message": "Tusk PHP: parsed-state dump written to the language server logs.",
				})
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	default:
		s.sendError(msg.ID, -32601, fmt.Sprintf("Unknown command: %s", params.Command))
	}
}

// getDocumentReader returns a function that reads document content by URI,
// falling back to disk if the document isn't open in the editor.
func (s *Server) getDocumentReader() func(string) string {
	return func(uri string) string {
		s.docMu.RLock()
		source, ok := s.documents[uri]
		s.docMu.RUnlock()
		if ok {
			return source
		}
		// Fall back to reading from disk
		path := symbols.URIToPath(uri)
		content, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return string(content)
	}
}

func (s *Server) getDocument(uri string) string {
	s.docMu.RLock()
	source := s.documents[uri]
	s.docMu.RUnlock()
	return source
}

func (s *Server) publishDiagnostics(uri, source string) {
	diagnostics := s.diag.Analyze(uri, source)
	if diagnostics == nil {
		diagnostics = []protocol.Diagnostic{}
	}
	s.sendNotification("textDocument/publishDiagnostics", map[string]interface{}{"uri": uri, "diagnostics": diagnostics})
}

// republishOpenDocuments re-runs diagnostics for every open document.
// Called after async workspace + composer indexing settles so any findings
// that depended on a populated index (or were soft-moded during warm-up)
// produce their authoritative result.
func (s *Server) republishOpenDocuments() {
	s.docMu.RLock()
	pairs := make([][2]string, 0, len(s.documents))
	for uri, source := range s.documents {
		pairs = append(pairs, [2]string{uri, source})
	}
	s.docMu.RUnlock()

	for _, p := range pairs {
		s.publishDiagnostics(p[0], p[1])
	}
}

// indexFileByURI indexes a file, using the IDE helper merge strategy for known IDE helper files.
func (s *Server) indexFileByURI(uri string, source string) {
	path := symbols.URIToPath(uri)
	if workspace.IsIDEHelperFile(path) {
		s.index.IndexIDEHelperFile(uri, source)
	} else {
		s.index.IndexFile(uri, source)
	}
	if s.routeIndex != nil {
		s.routeIndex.IndexFile(uri, source)
	}
}

