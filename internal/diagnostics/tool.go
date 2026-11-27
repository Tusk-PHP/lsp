package diagnostics

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const toolWaitDelay = 2 * time.Second

type externalTool struct {
	label    string
	binPath  string
	rootPath string
	timeout  time.Duration
	logger   *log.Logger
	enabled  bool
}

func newExternalTool(label, bin, rootPath, configuredPath string, enabled *bool, timeout time.Duration, logger *log.Logger) externalTool {
	t := externalTool{label: label, rootPath: rootPath, timeout: timeout, logger: logger, enabled: enabled == nil || *enabled}
	switch {
	case configuredPath != "":
		t.binPath = configuredPath
	case fileExists(filepath.Join(rootPath, "vendor", "bin", bin)):
		t.binPath = filepath.Join(rootPath, "vendor", "bin", bin)
	default:
		if path, err := exec.LookPath(bin); err == nil {
			t.binPath = path
		} else {
			t.enabled = false
		}
	}
	return t
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (t externalTool) available() bool {
	return t.enabled && t.binPath != ""
}

func (t externalTool) run(ctx context.Context, filePath string, args ...string) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, t.binPath, append(args, filePath)...)
	cmd.Dir = t.rootPath
	cmd.WaitDelay = toolWaitDelay
	stopProcessTreeOnCancel(cmd)
	output, err := cmd.Output()
	if err == nil {
		return output, true
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.logger.Printf("%s on %s stopped after %s, its timeout", t.label, filePath, t.timeout)
		return nil, false
	}
	if ctx.Err() != nil {
		return nil, false
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.logger.Printf("%s on %s could not run: %v", t.label, filePath, err)
		return nil, false
	}
	if exitErr.ExitCode() != 1 {
		t.logger.Printf("%s on %s failed (exit %d): %s", t.label, filePath, exitErr.ExitCode(), exitErr.Stderr)
		return nil, false
	}
	return output, true
}
