package lsp

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/composer/cardhover"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/coverage"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	"github.com/Tusk-PHP/lsp/internal/document"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/hover"
	"github.com/Tusk-PHP/lsp/internal/inlayhint"
	"github.com/Tusk-PHP/lsp/internal/models"
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
	coverage      *coverage.Store
	composerHover *cardhover.Provider
	inlayHint     *inlayhint.Provider
	diag          *diagnostics.Provider
	analyzer      *analyzer.Analyzer
	routeIndex    *frameworklaravel.RouteIndex
	schemaCache   *models.SchemaCache
	docMu         sync.RWMutex
	documents     map[string]string
	// blade keeps the compiled PHP of open Blade views (see blade.go).
	blade bladeViews
	// viewTypes types the variables Blade views receive (see blade_vars.go).
	viewTypes *frameworklaravel.ViewTypes
	// bladeFeatures answers requests about Blade itself: directives and the
	// views, sections and stacks they name.
	bladeFeatures *blade.Features
	// components finds the app's Blade components; forgotten when a
	// component or provider is saved.
	components *frameworklaravel.Components
	// providers reads what service providers register with Blade and the
	// view factory.
	providers *frameworklaravel.Providers
	// writeMu keeps frames whole on the output stream (see writeMessage).
	writeMu sync.Mutex
	// publishMu orders diagnostics publishes (see publishDiagnostics).
	publishMu sync.Mutex
	// docIndex orders background indexing of open documents.
	docIndex docIndexQueue
	// diagDebounce delays diagnostics after edits (see scheduleDiagnostics).
	diagDebounce diagDebouncer
	encoding     document.Encoding
	rootPath     string
	framework    string
	reader       *bufio.Reader
	writer       io.Writer
	logger       *log.Logger
	shutdown     bool
	strict       bool      // when true, recovered panics are re-raised after logging
	exitFunc     func(int) // called by the "exit" notification handler; defaults to os.Exit

	builtinPHPVersion string
	builtinPHPSource  string // "composer" | "local" | "fallback"

	clientSupportsShowDocument bool
	coverageNotifications      bool

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

// maxConsecutiveReadErrors is how many reads in a row can fail before Run stops.
const maxConsecutiveReadErrors = 100

func (s *Server) Run() error {
	s.logger.Println("PHP LSP server starting...")
	readErrors := 0
	for {
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			// A malformed message is worth skipping, but a stream that keeps failing
			// would otherwise spin here and log without end.
			readErrors++
			if readErrors >= maxConsecutiveReadErrors {
				return fmt.Errorf("giving up after %d read errors in a row: %w", readErrors, err)
			}
			s.logger.Printf("Read error: %v", err)
			continue
		}
		readErrors = 0
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
