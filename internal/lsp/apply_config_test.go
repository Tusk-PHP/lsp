package lsp

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func waitForLogMessage(t *testing.T, h *lspHarness, text string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case msg := <-h.notes:
			params, _ := msg["params"].(map[string]interface{})
			got, _ := params["message"].(string)
			if msg["method"] == "window/logMessage" && strings.Contains(got, text) {
				return
			}
		case <-deadline:
			t.Fatalf("no window/logMessage containing %q", text)
		}
	}
}

func TestConfiguredFrameworkAndComposerOpenOnDefinitionWithoutShowDocument(t *testing.T) {
	h, _ := startWatchedProject(t, map[string]string{
		"composer.json":  `{"require": {"php": "^8.1"}}`,
		".tusk-php.json": `{"framework": "symfony", "composer": {"openOnDefinition": true}}`,
	}, map[string]interface{}{}, nil)
	waitForLogMessage(t, h, "composer.openOnDefinition is enabled but the client does not advertise window/showDocument support")
	if h.server.framework != "symfony" {
		t.Errorf("a framework named in the configuration is used as it is, got %q", h.server.framework)
	}
}

func TestTheFileLimitWarningUsesTheCurrentMaxIndexFiles(t *testing.T) {
	h, _ := startWatchedProject(t, map[string]string{
		"composer.json": `{"require": {"php": "^8.1"}}`,
		"app/A.php":     "<?php\nclass A {}\n",
		"app/B.php":     "<?php\nclass B {}\n",
	}, map[string]interface{}{}, map[string]interface{}{"maxIndexFiles": 1})
	waitForLogMessage(t, h, workspace.MaxIndexFilesWarning(1))
}

func TestWatchedChangesWithoutAFilePathAreIgnored(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{"composer.json": `{"require": {"php": "^8.1"}}`}, map[string]interface{}{}, nil)
	<-h.server.indexed
	writeProjectFiles(t, root, map[string]string{"app/Invoice.php": "<?php\nnamespace App;\nclass Invoice {}\n"})
	h.notify("workspace/didChangeWatchedFiles", map[string]interface{}{"changes": []map[string]interface{}{
		{"uri": "untitled:Untitled-1", "type": 2},
		{"uri": "file://" + filepath.Join(root, "app/Invoice.php"), "type": 1},
	}})
	waitForSymbols(t, h, "Invoice", func(names []string) bool { return slices.Contains(names, "Invoice") })
}
