//go:build wasm

package main

import (
	"fmt"
	"io"
)

// runCheck isn't available in the wasm build, which has no workspace on disk
// to check.
func runCheck(args []string, stdout, stderr io.Writer) (int, error) {
	return 2, fmt.Errorf("the check subcommand is not supported in the wasm build")
}
