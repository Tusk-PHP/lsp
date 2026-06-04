package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/graph"
	"github.com/Tusk-PHP/lsp/internal/lsp"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

var (
	version    = "0.9.0"
	showVer    = flag.Bool("version", false, "Print version and exit")
	logFile    = flag.String("log", "", "Log file path (default: stderr)")
	transport  = flag.String("transport", "stdio", "Transport: stdio")
	stdioMode  = flag.Bool("stdio", false, "Use stdio transport")
	strictMode = flag.Bool("strict", false, "Strict mode: re-panic after recovering (also enabled via TUSK_STRICT env var)")
	parseFile  = flag.String("parse", "", "Parse a PHP file and print its AST as JSON, then exit")
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "graph" {
		if err := runGraph(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	flag.Parse()
	if *stdioMode {
		*transport = "stdio"
	}
	if *showVer {
		fmt.Printf("tusk-php %s\n", version)
		os.Exit(0)
	}
	if *parseFile != "" {
		src, err := os.ReadFile(*parseFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}
		result := parser.New().Parse(string(src))
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	// Strict mode is enabled via flag OR the TUSK_STRICT environment variable.
	strict := *strictMode || isTruthy(os.Getenv("TUSK_STRICT"))
	var logger *log.Logger
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open log: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		logger = log.New(f, "[tusk-php] ", log.LstdFlags|log.Lshortfile)
	} else {
		logger = log.New(os.Stderr, "[tusk-php] ", log.LstdFlags|log.Lshortfile)
	}
	logger.Printf("Starting tusk-php %s", version)
	lsp.ServerVersion = version
	server := lsp.NewServer(os.Stdin, os.Stdout, logger)
	server.SetStrict(strict)
	if err := server.Run(); err != nil {
		logger.Fatalf("Server error: %v", err)
	}
}

// isTruthy returns true when s is one of the common truthy env-var values
// ("1", "true", "yes", "on"), case-insensitively.
func isTruthy(s string) bool {
	switch s {
	case "1", "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON":
		return true
	}
	return false
}

// loadProjectMetadata loads config and detects the PHP builtin profile for a
// given root path. It mirrors the unexported helper in internal/mcpserver.
// config.LoadFromFile returns a default-merged config on missing file, so no
// special not-exist handling is needed here.
func loadProjectMetadata(rootPath string) (*config.Config, string, symbols.BuiltinProfile, error) {
	cfgPath := filepath.Join(rootPath, ".tusk-php.json")
	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil {
		return nil, "", symbols.BuiltinProfile{}, err
	}

	framework := cfg.Framework
	if framework == "" || framework == "auto" {
		framework = config.DetectFramework(rootPath)
	}

	timeout := time.Duration(cfg.PHPDetectTimeoutMs) * time.Millisecond
	profile, _ := workspace.ResolveBuiltinProfile(rootPath, cfg.PHPBinary, timeout, nil)
	return cfg, framework, profile, nil
}

// runGraph implements the `graph` subcommand.
func runGraph(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: tusk-php graph <kind> [flags] (supported: container)")
	}
	if args[0] != "container" {
		return fmt.Errorf("unknown graph kind %q (supported: container)", args[0])
	}

	fs := flag.NewFlagSet("graph container", flag.ExitOnError)
	depsFlag := fs.String("deps", "none", "Dependency mode: none|boundary|full")
	formatFlag := fs.String("format", "json", "Output format: json|mermaid|dot")
	rootFlag := fs.String("root", "", "Project root (default: current directory)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	// Resolve root directory.
	root := *rootFlag
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("unable to determine working directory: %w", err)
		}
	}

	// Validate --deps.
	switch *depsFlag {
	case "none", "boundary", "full":
		// valid
	default:
		return fmt.Errorf("invalid --deps %q: must be one of none, boundary, full", *depsFlag)
	}

	// Validate --format.
	switch *formatFlag {
	case "json", "mermaid", "dot":
		// valid
	default:
		return fmt.Errorf("invalid --format %q: must be one of json, mermaid, dot", *formatFlag)
	}

	cfg, framework, profile, err := loadProjectMetadata(root)
	if err != nil {
		return fmt.Errorf("loading project metadata: %w", err)
	}

	// Use stderr for the build logger so stdout stays clean for graph output.
	logger := log.New(os.Stderr, "[tusk-php graph] ", log.LstdFlags)

	ws, err := workspace.Build(context.Background(), workspace.Options{
		RootPath:       root,
		Framework:      framework,
		Config:         cfg,
		Logger:         logger,
		BuiltinProfile: profile,
	})
	if err != nil {
		return fmt.Errorf("building workspace: %w", err)
	}

	var pkgs graph.PackageResolver = composer.NewPackageResolver(root)
	mode := graph.DepsMode(*depsFlag)

	g := graph.BuildContainer(ws.Index, ws.Container, graph.Options{
		Deps:     mode,
		Packages: pkgs,
	})

	switch *formatFlag {
	case "json":
		return g.EncodeJSON(os.Stdout)
	case "mermaid":
		fmt.Fprintln(os.Stdout, graph.RenderMermaid(g))
	case "dot":
		fmt.Fprintln(os.Stdout, graph.RenderDOT(g))
	}
	return nil
}
