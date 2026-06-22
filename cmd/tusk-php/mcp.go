//go:build !wasm

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

// runMCP implements the `mcp` subcommand, folding the former tusk-mcp binary
// into the main tusk-php binary. It dispatches to the `dump` sub-subcommand
// when args[0] == "dump", and otherwise starts the MCP server over stdio.
func runMCP(args []string) {
	mcpserver.ServerVersion = version

	if len(args) > 0 && args[0] == "dump" {
		runMCPDump(args[1:])
		return
	}

	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	var (
		showVersion bool
		rootPath    string
	)
	fs.BoolVar(&showVersion, "version", false, "show version")
	fs.StringVar(&rootPath, "root", "", "workspace root path (defaults to cwd)")
	_ = fs.Parse(args)

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

// runMCPDump implements the `mcp dump` sub-subcommand, writing an AI context
// pack to the output directory.
func runMCPDump(args []string) {
	fs := flag.NewFlagSet("mcp dump", flag.ExitOnError)
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
