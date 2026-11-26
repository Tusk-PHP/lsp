//go:build !wasm

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func introspectOutput(t *testing.T, args ...string) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	runErr := runIntrospect(args)
	os.Stdout = stdout
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if runErr != nil {
		t.Fatalf("runIntrospect(%v): %v", args, runErr)
	}
	return string(out)
}

func TestIntrospectReportsTheProjectsPHPProfile(t *testing.T) {
	root := writeProject(t, map[string]string{
		"composer.json": `{"require": {"php": "^7.4"}}`,
		"a.php":         "<?php\nfunction a() {}\n",
	})
	if out := introspectOutput(t, filepath.Join(root, "a.php")); !strings.Contains(out, "PHP 7.4") {
		t.Errorf("introspect reports the project's PHP version:\n%s", out)
	}
}
