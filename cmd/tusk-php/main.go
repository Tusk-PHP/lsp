package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Tusk-PHP/lsp/internal/lsp"
	"github.com/Tusk-PHP/lsp/internal/parser"
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
