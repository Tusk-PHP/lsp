//go:build unix

package diagnostics

import (
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

func TestStopToolsStopsEveryRunInProgress(t *testing.T) {
	f := newToolFixture(t, 0)
	f.block(t)
	started := time.Now()
	run := f.runInBackground()
	f.waitUntilStarted(t)
	f.p.StopTools()
	if receive(t, run, time.Second) {
		t.Error("a run StopTools stopped is reported as stopped")
	}
	if got := f.phpstanMessages(); len(got) != 0 {
		t.Errorf("a stopped run keeps no findings, got %v", got)
	}
	f.assertWorkerKilled(t, started)
}

func TestKillingAProcessGroupThatIsGoneReportsTheProcessDone(t *testing.T) {
	cmd := exec.Command("true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if err := killProcessGroup(cmd.Process.Pid); !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("a process group that already ended is done, got %v", err)
	}
}

func writeScript(t *testing.T, dir, name, script string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func toolsProvider(t *testing.T, phpstan, pint string) (*Provider, *strings.Builder) {
	t.Helper()
	p := newTestProvider()
	logs := &strings.Builder{}
	p.logger = log.New(logs, "", 0)
	cfg := *p.settings.Load().cfg
	enabled := true
	cfg.PHPStanEnabled, cfg.PHPStanPath = &enabled, phpstan
	cfg.PintEnabled, cfg.PintPath = &enabled, pint
	p.SetConfig(&cfg)
	return p, logs
}

func TestToolsThatFailOrCannotStartReportNothingAndSayWhy(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "Order.php")
	if err := os.WriteFile(file, []byte("<?php\nstrlen(1);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pint := writeScript(t, dir, "pint", "echo 'config is broken' >&2\nexit 2\n")
	p, logs := toolsProvider(t, filepath.Join(dir, "missing-phpstan"), pint)
	if !p.RunTools("file://"+file, file, unchanged) {
		t.Fatal("a run whose tools fail still ends")
	}
	if diags := p.Analyze("file://"+file, "<?php\nstrlen(1);\n"); len(filterBySource(diags, "phpstan")) != 0 || len(filterBySource(diags, "pint")) != 0 {
		t.Errorf("tools that fail report nothing, got %+v", diags)
	}
	if got := logs.String(); !strings.Contains(got, "PHPStan on "+file+" could not run") || !strings.Contains(got, "Pint on "+file+" failed (exit 2): config is broken") {
		t.Errorf("the log says why each tool reported nothing, got %q", got)
	}
}

func TestAToolThatExitsCleanlyReportsItsOutput(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "Order.php")
	if err := os.WriteFile(file, []byte("<?php\nstrlen(1);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	phpstan := writeScript(t, dir, "phpstan", "cat <<'JSON'\n"+phpstanFinding(file, "Parameter #1 expects string")+"\nJSON\n")
	p, _ := toolsProvider(t, phpstan, "")
	p.RunTools("file://"+file, file, unchanged)
	if stan := filterBySource(p.Analyze("file://"+file, "<?php\nstrlen(1);\n"), "phpstan"); len(stan) != 1 {
		t.Errorf("the finding of a run that exits 0 is reported, got %+v", stan)
	}
}

func TestToolsAreFoundInVendorBinThenOnThePath(t *testing.T) {
	root := t.TempDir()
	vendorBin := filepath.Join(root, "vendor", "bin")
	if err := os.MkdirAll(vendorBin, 0o755); err != nil {
		t.Fatal(err)
	}
	vendored := writeScript(t, vendorBin, "phpstan", "exit 0\n")
	path := t.TempDir()
	onPath := writeScript(t, path, "pint", "exit 0\n")
	t.Setenv("PATH", path)
	logger := log.New(io.Discard, "", 0)

	if tool := newExternalTool("PHPStan", "phpstan", root, "", nil, time.Second, logger); !tool.available() || tool.binPath != vendored {
		t.Errorf("the project's vendor/bin tool is used, got %+v", tool)
	}
	if tool := newExternalTool("Pint", "pint", root, "", nil, time.Second, logger); !tool.available() || tool.binPath != onPath {
		t.Errorf("a tool on the PATH is used when vendor/bin has none, got %+v", tool)
	}
	if tool := newExternalTool("Psalm", "psalm", root, "", nil, time.Second, logger); tool.available() {
		t.Errorf("a tool found nowhere is unavailable, got %+v", tool)
	}
}
