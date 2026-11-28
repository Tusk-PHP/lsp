//go:build !wasm

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandsRejectARootThatIsNoDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "A.php")
	if err := os.WriteFile(file, []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "gone")
	for name, run := range map[string]func() error{
		"introspect":      func() error { return runIntrospect([]string{"--root", missing, file}) },
		"introspect file": func() error { return runIntrospect([]string{filepath.Join(dir, "B.php")}) },
		"deps unused":     func() error { return runDepsUnused([]string{"--root", missing}) },
		"deps tree":       func() error { return runDepsTree([]string{"--root", file}) },
		"deps usage":      func() error { return runDepsUsage([]string{"--root", missing}) },
	} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "resolving") {
			t.Errorf("%s: %v, want an error resolving the path", name, err)
		}
	}
}
