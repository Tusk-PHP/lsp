//go:build wasm

package main

import (
	"fmt"
	"os"
)

// runMCP is a no-op stub for the wasm build. The MCP SDK is not available
// under wasip1/wasm, so this subcommand exits with an informative error.
func runMCP(args []string) {
	fmt.Fprintln(os.Stderr, "mcp subcommand is not supported in the wasm build")
	os.Exit(1)
}
