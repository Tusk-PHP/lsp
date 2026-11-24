package mcpserver

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectMetadataLogsSkippedConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(`{"diagnosticIgnore":[{"message":"("}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	if _, _, _, _, err := loadProjectMetadata(root, slog.New(slog.NewTextHandler(&logs, nil))); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "skipped part of the configuration") || !strings.Contains(logs.String(), "diagnosticIgnore message") {
		t.Errorf("logs: %s", logs.String())
	}
}
