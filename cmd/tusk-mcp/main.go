package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Tusk-PHP/lsp/internal/mcpserver"
)

var version = "0.5.0"

func main() {
	mcpserver.ServerVersion = version

	if len(os.Args) > 1 && os.Args[1] == "dump" {
		runDump(os.Args[2:])
		return
	}

	var (
		showVersion bool
		rootPath    string
	)
	flag.BoolVar(&showVersion, "version", false, "show version")
	flag.StringVar(&rootPath, "root", "", "workspace root path (defaults to cwd)")
	flag.Parse()

	if showVersion {
		fmt.Printf("%s %s\n", mcpserver.ServerName, version)
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	svc, err := mcpserver.New(context.Background(), rootPath, logger)
	if err != nil {
		logger.Error("failed to initialize MCP server", "error", err)
		os.Exit(1)
	}
	if err := svc.Run(context.Background()); err != nil {
		logger.Error("MCP server stopped with error", "error", err)
		os.Exit(1)
	}
}

func runDump(args []string) {
	fs := flag.NewFlagSet("dump", flag.ExitOnError)
	var (
		rootPath string
		outDir   string
	)
	fs.StringVar(&rootPath, "root", "", "workspace root path (defaults to cwd)")
	fs.StringVar(&outDir, "out", filepath.Join(".tusk", "ai-context"), "output directory for context pack files")
	_ = fs.Parse(args)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	svc, err := mcpserver.New(context.Background(), rootPath, logger)
	if err != nil {
		logger.Error("failed to initialize MCP dump", "error", err)
		os.Exit(1)
	}
	if err := svc.Dump(context.Background(), outDir); err != nil {
		logger.Error("failed to write MCP dump", "error", err)
		os.Exit(1)
	}
}
