package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/composer/lockfile"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/deps"
	"github.com/Tusk-PHP/lsp/internal/graph"
	"github.com/Tusk-PHP/lsp/internal/introspect"
	"github.com/Tusk-PHP/lsp/internal/lsp"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/session"
	"github.com/Tusk-PHP/lsp/internal/strict"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/unuseddeps"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

var (
	version    = "0.17.0"
	showVer    = flag.Bool("version", false, "Print version and exit")
	logFile    = flag.String("log", "", "Log file path (default: stderr)")
	transport  = flag.String("transport", "stdio", "Transport: stdio")
	stdioMode  = flag.Bool("stdio", false, "Use stdio transport")
	strictMode = flag.Bool("strict", false, "Strict mode: re-panic after recovering (also enabled via TUSK_STRICT env var)")
	parseFile  = flag.String("parse", "", "Parse a PHP file and print its AST as JSON, then exit")
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "deps" {
		if err := runDeps(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "introspect" {
		if err := runIntrospect(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "check" {
		code, err := runCheck(os.Args[2:], os.Stdout, os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(code)
	}
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		runMCP(os.Args[2:])
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
	if *strictMode {
		if err := os.Setenv(strict.EnvVar, "1"); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to enable strict mode: %v\n", err)
			os.Exit(1)
		}
	}
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
	server := lsp.NewServer(serverStdin(), os.Stdout, logger)
	server.SetStrict(strict.Enabled())
	if err := server.Run(); err != nil {
		logger.Fatalf("Server error: %v", err)
	}
}

// loadProjectMetadata loads a root's configuration, framework and PHP
// profile, reporting the configuration's problems on stderr.
func loadProjectMetadata(rootPath string, stderr io.Writer) (*config.Config, string, symbols.BuiltinProfile, error) {
	project, err := workspace.LoadProject(rootPath, nil)
	if err != nil {
		return nil, "", symbols.BuiltinProfile{}, err
	}
	for _, problem := range project.Config.Problems() {
		fmt.Fprintln(stderr, "tusk-php: skipped part of the configuration:", problem)
	}
	return project.Config, project.Framework, project.Profile, nil
}

// emitGraph renders g to stdout according to format (json/mermaid/dot).
func emitGraph(g *graph.Graph, format string) error {
	switch format {
	case "json":
		return g.EncodeJSON(os.Stdout)
	case "mermaid":
		fmt.Fprintln(os.Stdout, graph.RenderMermaid(g))
	case "dot":
		fmt.Fprintln(os.Stdout, graph.RenderDOT(g))
	}
	return nil
}

// buildWorkspace loads project metadata and builds a fully-indexed workspace for
// the given root directory. The logger prefix is used for the build logger that
// writes to stderr. Both runDeps and runIntrospect use this shared helper so that
// the workspace-build logic lives in one place.
func buildWorkspace(root, logPrefix string) (*workspace.Bootstrapped, string, error) {
	cfg, framework, profile, err := loadProjectMetadata(root, os.Stderr)
	if err != nil {
		return nil, framework, fmt.Errorf("loading project metadata: %w", err)
	}

	// Use stderr for the build logger so stdout stays clean for command output.
	logger := log.New(os.Stderr, "["+logPrefix+"] ", log.LstdFlags)

	ws, err := workspace.Build(context.Background(), workspace.Options{
		RootPath:       root,
		Framework:      framework,
		Config:         cfg,
		Logger:         logger,
		BuiltinProfile: profile,
	})
	if err != nil {
		return nil, framework, fmt.Errorf("building workspace: %w", err)
	}
	return ws, framework, nil
}

// runIntrospect implements the `introspect` subcommand.
func runIntrospect(args []string) error {
	fs := flag.NewFlagSet("introspect", flag.ContinueOnError)
	rootFlag := fs.String("root", "", "Project root (default: directory containing <file>)")
	verbosityFlag := fs.String("verbosity", "compact", "Verbosity level: compact|full")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Require exactly one positional argument: the file to introspect.
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: tusk-php introspect <file> [--root=.] [--verbosity=compact|full]")
	}
	filePath := fs.Arg(0)

	// Validate --verbosity.
	switch *verbosityFlag {
	case "compact", "full":
		// valid
	default:
		return fmt.Errorf("invalid --verbosity %q: must be one of compact, full", *verbosityFlag)
	}

	// Resolve the file to an absolute path.
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("resolving file path: %w", err)
	}

	// Determine the project root: use --root if given, otherwise the directory
	// containing the file (mirrors runDeps convention).
	root := *rootFlag
	if root == "" {
		root = filepath.Dir(absFile)
	} else {
		root, err = filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("resolving root path: %w", err)
		}
	}

	ws, framework, err := buildWorkspace(root, "tusk-php introspect")
	if err != nil {
		return err
	}

	// Read the file source from disk.
	srcBytes, err := os.ReadFile(absFile)
	if err != nil {
		return fmt.Errorf("reading file %s: %w", absFile, err)
	}
	source := string(srcBytes)

	uri := symbols.PathToURI(absFile)

	// Run the native rules synchronously, wired as the server wires them, for
	// a self-consistent dump.
	findings := session.New(ws, session.Options{}).NativeFindings(source)

	v := introspect.ParseVerbosity(*verbosityFlag)
	in := introspect.Input{
		URI:           uri,
		Source:        source,
		Index:         ws.Index,
		Container:     ws.Container,
		Framework:     framework,
		ServerVersion: version,
		Profile:       introspect.FormatProfile(ws.Profile().PHPVersion, ws.Profile().Extensions),
		BufferState:   "disk",
		Diagnostics:   findings,
		Verbosity:     v,
	}
	fmt.Print(introspect.Render(introspect.Document(in), v))
	return nil
}

// runDeps implements the `deps` subcommand.
func runDeps(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: tusk-php deps <subcommand> [flags] (supported: unused, tree, usage)")
	}
	sub := args[0]
	switch sub {
	case "unused":
		return runDepsUnused(args[1:])
	case "tree":
		return runDepsTree(args[1:])
	case "usage":
		return runDepsUsage(args[1:])
	default:
		return fmt.Errorf("unknown deps subcommand %q (supported: unused, tree, usage)", sub)
	}
}

// runDepsUnused is the original `deps unused` handler, preserved exactly.
func runDepsUnused(args []string) error {
	fs := flag.NewFlagSet("deps unused", flag.ContinueOnError)
	rootFlag := fs.String("root", "", "Project root (default: current directory)")
	formatFlag := fs.String("format", "text", "Output format: text|json")
	if err := fs.Parse(args); err != nil {
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

	// Validate --format.
	switch *formatFlag {
	case "text", "json":
		// valid
	default:
		return fmt.Errorf("invalid --format %q: must be one of text, json", *formatFlag)
	}

	ws, framework, err := buildWorkspace(root, "tusk-php deps")
	if err != nil {
		return err
	}

	rep := unuseddeps.Analyze(ws.Index, root, framework)

	switch *formatFlag {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	default: // text
		fmt.Fprintf(os.Stdout, "%d dependencies checked, %d candidates (framework: %s)\n",
			rep.Checked, len(rep.Candidates), rep.Framework)
		if len(rep.Candidates) == 0 {
			fmt.Fprintln(os.Stdout, "no unused dependency candidates found")
		}
		for _, c := range rep.Candidates {
			fmt.Fprintf(os.Stdout, "  %s — %s\n", c.Package, c.Reason)
		}
	}
	return nil
}

// runDepsTree implements `deps tree`.
func runDepsTree(args []string) error {
	fs := flag.NewFlagSet("deps tree", flag.ContinueOnError)
	rootFlag := fs.String("root", "", "Project root (default: current directory)")
	depthFlag := fs.Int("depth", -1, "Maximum traversal depth (<0 = unlimited)")
	fullFlag := fs.Bool("full", false, "Expand all transitive dependencies regardless of --depth")
	formatFlag := fs.String("format", "mermaid", "Output format: mermaid|dot|json")
	if err := fs.Parse(args); err != nil {
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

	// Validate --format.
	switch *formatFlag {
	case "mermaid", "dot", "json":
		// valid
	default:
		return fmt.Errorf("invalid --format %q: must be one of mermaid, dot, json", *formatFlag)
	}

	lf, err := lockfile.Load(root)
	if err != nil {
		return fmt.Errorf("loading composer.lock: %w", err)
	}
	if lf == nil {
		return fmt.Errorf("no composer.lock found at %s", root)
	}

	direct := composer.DirectRequires(root)
	g := deps.BuildTree(lf, direct, deps.TreeOptions{Depth: *depthFlag, Full: *fullFlag})
	return emitGraph(g, *formatFlag)
}

// runDepsUsage implements `deps usage`.
func runDepsUsage(args []string) error {
	fs := flag.NewFlagSet("deps usage", flag.ContinueOnError)
	rootFlag := fs.String("root", "", "Project root (default: current directory)")
	formatFlag := fs.String("format", "text", "Output format: text|json")
	verboseFlag := fs.Bool("verbose", false, "Include touched/exposed class counts and version in text output")
	if err := fs.Parse(args); err != nil {
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

	// Validate --format.
	switch *formatFlag {
	case "text", "json":
		// valid
	default:
		return fmt.Errorf("invalid --format %q: must be one of text, json", *formatFlag)
	}

	ws, _, err := buildWorkspace(root, "tusk-php deps")
	if err != nil {
		return err
	}

	rows := deps.Usage(ws.Index, root)

	switch *formatFlag {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	default: // text
		if len(rows) == 0 {
			fmt.Fprintln(os.Stdout, "no packages with a PSR-4 surface found")
			return nil
		}
		fmt.Fprintf(os.Stdout, "%d packages analysed (briefing-grade; no code executed)\n", len(rows))
		for _, r := range rows {
			if *verboseFlag {
				ver := ""
				if r.Version != "" {
					ver = ", v" + r.Version
				}
				fmt.Fprintf(os.Stdout, "  %s  %.1f%% (%d/%d classes touched%s)\n",
					r.Package, r.Percent, r.TouchedClasses, r.ExposedClasses, ver)
			} else {
				fmt.Fprintf(os.Stdout, "  %s  %.1f%%\n", r.Package, r.Percent)
			}
		}
	}
	return nil
}
