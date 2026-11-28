//go:build unix

package session

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/diagnostics"
)

func TestRunToolsKeepsFindingsUntilStopToolsStopsARun(t *testing.T) {
	s := newSession(t, Options{})
	dir := t.TempDir()
	file := filepath.Join(dir, "Order.php")
	if err := os.WriteFile(file, []byte("<?php\nstrlen(1);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := "file://" + file
	phpstan := filepath.Join(dir, "phpstan")
	finding := `{"totals": {"errors": 0, "file_errors": 1}, "files": {"` + file + `": {"errors": 1, "messages": [{"message": "Parameter #1 expects string", "line": 2, "identifier": "argument.type"}]}}}`
	script := "#!/bin/sh\nd=$(dirname \"$0\")\nif [ -e \"$d/block\" ]; then\n  touch \"$d/started\"\n  exec sleep 30\nfi\ncat <<'JSON'\n" + finding + "\nJSON\nexit 1\n"
	if err := os.WriteFile(phpstan, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := *s.Config()
	enabled, disabled := true, false
	cfg.PHPStanEnabled, cfg.PHPStanPath, cfg.PintEnabled = &enabled, phpstan, &disabled
	s.SetConfig(&cfg)
	current := func() bool { return true }

	if !s.RunTools(uri, file, current) {
		t.Fatal("a run that ends is reported")
	}
	diags := s.Analyze(uri, "<?php\nstrlen(1);\n")
	if !slices.ContainsFunc(diags, func(d diagnostics.Diagnostic) bool { return d.Source == "phpstan" && d.Code == "argument.type" }) {
		t.Fatalf("the run's PHPStan finding is reported, got %v", codes(diags))
	}

	if err := os.WriteFile(filepath.Join(dir, "block"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	result := make(chan bool, 1)
	go func() { result <- s.RunTools(uri, file, current) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "started")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the blocking run never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.StopTools()
	select {
	case kept := <-result:
		if kept {
			t.Error("a run StopTools stopped is reported as stopped")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StopTools didn't stop the run")
	}
}
