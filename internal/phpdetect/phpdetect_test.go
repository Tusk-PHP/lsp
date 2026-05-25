package phpdetect

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeScript writes a small shell script to dir/name with the given body
// and makes it executable. It returns the absolute path to the script.
func writeFakeScript(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	content := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("writeFakeScript: %v", err)
	}
	return path
}

func TestDetectParsesValidOutput(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeScript(t, dir, "php", `echo 8.2`)

	got, err := Detect(bin)
	if err != nil {
		t.Fatalf("Detect returned unexpected error: %v", err)
	}
	want := LocalPHP{Version: "8.2", Source: "local"}
	if got != want {
		t.Fatalf("Detect = %+v, want %+v", got, want)
	}
}

func TestDetectRejectsMalformedOutput(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeScript(t, dir, "php", `echo "not a version"`)

	got, err := Detect(bin)
	if err == nil {
		t.Fatalf("expected error for malformed output, got nil (Version=%q)", got.Version)
	}
	if got.Version != "" {
		t.Errorf("expected empty Version on error, got %q", got.Version)
	}
}

func TestDetectFailsWhenBinaryMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent-php")

	_, err := Detect(path)
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

func TestDetectTimesOut(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeScript(t, dir, "php", `sleep 2`)

	start := time.Now()
	_, err := Detect(bin)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error due to timeout, got nil")
	}
	if elapsed >= 1*time.Second {
		t.Errorf("Detect took %v; expected < 1s (500ms timeout + scheduling slack)", elapsed)
	}
}

func TestDetectEmptyBinaryPathDefaultsToPath(t *testing.T) {
	got, err := Detect("")
	if err != nil {
		// Acceptable: php not on $PATH in CI. Verify the error is exec-related.
		if !strings.Contains(err.Error(), "executable file not found") &&
			!strings.Contains(err.Error(), "exec") &&
			!errors.Is(err, os.ErrNotExist) {
			t.Errorf("unexpected error kind for missing php: %v", err)
		}
		return
	}
	// php was found; Version must be non-empty and Source must be "local".
	if got.Version == "" {
		t.Error("expected non-empty Version when php is found on PATH")
	}
	if got.Source != "local" {
		t.Errorf("expected Source %q, got %q", "local", got.Source)
	}
}
