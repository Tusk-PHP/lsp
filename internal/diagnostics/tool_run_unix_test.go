//go:build unix

package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const workerLifetime = 2 * time.Second

func phpstanFinding(file, message string) string {
	return `{"totals": {"errors": 0, "file_errors": 1}, "files": {"` + file + `": {"errors": 1, "messages": [{"message": "` + message + `", "line": 2, "identifier": "argument.type"}]}}}`
}

func writeBlockingTool(t *testing.T, dir, name, output string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nd=$(dirname \"$0\")\nif [ -e \"$d/block\" ]; then\n  rm \"$d/block\"\n  ( sleep 2; touch \"$d/survived\" ) &\n  touch \"$d/started\"\n  wait\nfi\ncat <<'JSON'\n" + output + "\nJSON\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

type toolFixture struct {
	dir, file, uri string
	p              *Provider
}

func newToolFixture(t *testing.T, timeoutMs int) toolFixture {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "Order.php")
	if err := os.WriteFile(file, []byte("<?php\nstrlen(1);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := newTestProvider()
	cfg := *p.settings.Load().cfg
	enabled := true
	cfg.PHPStanEnabled = &enabled
	cfg.PHPStanPath = writeBlockingTool(t, dir, "phpstan", phpstanFinding(file, "Parameter #1 expects string"))
	cfg.PHPStanTimeoutMs = timeoutMs
	p.SetConfig(&cfg)
	return toolFixture{dir: dir, file: file, uri: "file://" + file, p: p}
}

func (f toolFixture) block(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, "block"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f toolFixture) runInBackground() <-chan bool {
	result := make(chan bool, 1)
	go func() { result <- f.p.RunTools(f.uri, f.file, unchanged) }()
	return result
}

func (f toolFixture) waitUntilStarted(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !exists(filepath.Join(f.dir, "started")) {
		if time.Now().After(deadline) {
			t.Fatal("the fake tool never started its worker")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (f toolFixture) assertWorkerKilled(t *testing.T, since time.Time) {
	t.Helper()
	time.Sleep(time.Until(since.Add(workerLifetime + 500*time.Millisecond)))
	if exists(filepath.Join(f.dir, "survived")) {
		t.Error("the tool's worker process outlived the stopped run")
	}
}

func (f toolFixture) phpstanMessages() []string {
	var messages []string
	for _, d := range filterBySource(f.p.Analyze(f.uri, "<?php\nstrlen(1);\n"), "phpstan") {
		messages = append(messages, d.Message)
	}
	return messages
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func receive(t *testing.T, result <-chan bool, within time.Duration) bool {
	t.Helper()
	select {
	case kept := <-result:
		return kept
	case <-time.After(within):
		t.Fatalf("the run did not end within %s", within)
		return false
	}
}

func TestNewSaveStopsTheToolRunInProgress(t *testing.T) {
	f := newToolFixture(t, 0)
	f.block(t)
	started := time.Now()
	first := f.runInBackground()
	f.waitUntilStarted(t)

	if !f.p.RunTools(f.uri, f.file, unchanged) {
		t.Error("the newest run ends and is reported")
	}
	if receive(t, first, time.Second) {
		t.Error("the run a newer save replaced is reported as stopped")
	}
	if got := f.phpstanMessages(); len(got) != 1 {
		t.Errorf("the newest run's finding is kept once, got %v", got)
	}
	f.assertWorkerKilled(t, started)
}

func TestToolRunIsKilledAtItsTimeout(t *testing.T) {
	f := newToolFixture(t, 200)
	f.block(t)
	started := time.Now()
	f.p.RunTools(f.uri, f.file, unchanged)
	if elapsed := time.Since(started); elapsed >= workerLifetime {
		t.Errorf("a run past its 200ms timeout is stopped, it took %s", elapsed)
	}
	if got := f.phpstanMessages(); len(got) != 0 {
		t.Errorf("a run stopped at its timeout reports nothing, got %v", got)
	}
	f.assertWorkerKilled(t, started)
}

func TestClosingAFileStopsItsToolRun(t *testing.T) {
	f := newToolFixture(t, 0)
	f.block(t)
	run := f.runInBackground()
	f.waitUntilStarted(t)
	f.p.ClearCache(f.uri)
	if receive(t, run, time.Second) {
		t.Error("a run of a closed file is reported as stopped")
	}
	if got := f.phpstanMessages(); len(got) != 0 {
		t.Errorf("a closed file keeps no tool findings, got %v", got)
	}
}

func TestToolFindingsOfAFileChangedMeanwhileAreDropped(t *testing.T) {
	f := newToolFixture(t, 0)
	if !f.p.RunTools(f.uri, f.file, unchanged) {
		t.Fatal("the first run ends")
	}
	writeTool(t, f.dir, "phpstan", phpstanFinding(f.file, "Finding of a version the editor no longer has"))
	if !f.p.RunTools(f.uri, f.file, func() bool { return false }) {
		t.Error("a run that ended is reported even when its findings are dropped")
	}
	if got := f.phpstanMessages(); len(got) != 1 || got[0] != "Parameter #1 expects string" {
		t.Errorf("findings worked out for a file that changed meanwhile are dropped, got %v", got)
	}
}
