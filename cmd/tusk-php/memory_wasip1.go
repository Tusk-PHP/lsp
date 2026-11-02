//go:build wasip1

package main

import "runtime/debug"

// The VS Code web extension stops the module at 512 MB, so have the garbage
// collector work harder before it gets there rather than running out.
const wasmMemoryLimit = 448 << 20

func init() {
	debug.SetMemoryLimit(wasmMemoryLimit)
}
