// Package phpdetect detects the PHP version of a locally installed PHP
// binary. It is used as a fallback when composer.json does not declare a
// platform PHP version. The detection result is intended to be cached for
// the lifetime of an LSP session.
package phpdetect

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// versionRE matches exactly MAJOR.MINOR, e.g. "8.2".
var versionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// LocalPHP holds the result of a successful local PHP version detection.
type LocalPHP struct {
	// Version is the detected major.minor version string, e.g. "8.2".
	Version string
	// Source is a free-form provenance string. Detect always sets this to
	// "local".
	Source string
}

// Detect runs the PHP binary at binaryPath with a 500 ms deadline and returns
// its major.minor version. If binaryPath is empty, "php" is used so that the
// user's $PATH is consulted. Any execution failure, non-zero exit, context
// cancellation, or malformed output causes a non-nil error to be returned;
// Version will be empty in that case.
func Detect(binaryPath string) (LocalPHP, error) {
	if binaryPath == "" {
		binaryPath = "php"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "-r", `echo PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION;`)
	// WaitDelay ensures that dangling child processes (e.g. a sleep spawned by
	// the shell) cannot block Output() beyond the context deadline. It is set
	// to 100 ms to give the OS time to propagate the kill before we forcibly
	// close the pipes.
	cmd.WaitDelay = 100 * time.Millisecond

	out, err := cmd.Output()
	if err != nil {
		return LocalPHP{}, fmt.Errorf("phpdetect: running %q: %w", binaryPath, err)
	}

	version := strings.TrimSpace(string(out))
	if !versionRE.MatchString(version) {
		return LocalPHP{}, fmt.Errorf("phpdetect: unexpected output %q from %q (want MAJOR.MINOR)", version, binaryPath)
	}

	return LocalPHP{Version: version, Source: "local"}, nil
}
