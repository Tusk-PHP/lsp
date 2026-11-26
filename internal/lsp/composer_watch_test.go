package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func startComposerProject(t *testing.T) (*lspHarness, string) {
	t.Helper()
	return startWatchedProject(t, map[string]string{
		"composer.json":                        `{"name": "app/app"}`,
		"vendor/composer/installed.json":       `{"packages": [{"name": "acme/lib", "install-path": "../acme/lib", "autoload": {"psr-4": {"Acme\\": "src/"}}}]}`,
		"vendor/acme/lib/src/Widget.php":       "<?php\nnamespace Acme;\nclass Widget {}\n",
		"vendor/acme/lib/src/Factory.php":      "<?php\nnamespace Acme;\nclass Factory {}\n",
		"vendor/acme/lib/src/Discontinued.php": "<?php\nnamespace Acme;\nclass Discontinued {}\n",
	}, watchingClient(), nil)
}

func waitForLog(t *testing.T, h *lspHarness, prefix string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case msg := <-h.notes:
			params, _ := msg["params"].(map[string]interface{})
			if text, _ := params["message"].(string); msg["method"] == "window/logMessage" && strings.HasPrefix(text, prefix) {
				return
			}
		case <-deadline:
			t.Fatalf("timeout waiting for the log message %q", prefix)
		}
	}
}

func TestComposerChangeReindexesVendorFiles(t *testing.T) {
	h, root := startComposerProject(t)
	waitForLog(t, h, "PHP LSP: Indexed 3 vendor files")

	writeProjectFiles(t, root, map[string]string{
		"vendor/acme/lib/src/Factory.php": "<?php\nnamespace Acme;\nclass Factory { public function make(): Widget { return new Widget(); } }\n",
		"vendor/acme/lib/src/Gadget.php":  "<?php\nnamespace Acme;\nclass Gadget extends Widget {}\n",
	})
	if err := os.Remove(filepath.Join(root, "vendor/acme/lib/src/Discontinued.php")); err != nil {
		t.Fatal(err)
	}
	h.notify("workspace/didChangeWatchedFiles", map[string]interface{}{
		"changes": []map[string]interface{}{
			{"uri": symbols.PathToURI(filepath.Join(root, "vendor/acme/lib/src/Other.json")), "type": 2},
			{"uri": symbols.PathToURI(filepath.Join(root, "vendor/composer/installed.json")), "type": 2},
		},
	})
	waitForLog(t, h, "PHP LSP: Re-indexed 3 vendor files after a Composer change")

	widget := symbols.PathToURI(filepath.Join(root, "vendor/acme/lib/src/Widget.php"))
	h.notify("textDocument/didOpen", map[string]interface{}{"textDocument": map[string]interface{}{
		"uri": widget, "languageId": "php", "version": 1, "text": "<?php\nnamespace Acme;\nclass Widget {}\n",
	}})
	var locs []testLocation
	request(t, h, "textDocument/references", widget, map[string]interface{}{"line": 2, "character": 7},
		map[string]interface{}{"context": map[string]interface{}{"includeDeclaration": true}}, &locs)
	byFile := map[string]int{}
	for _, loc := range locs {
		byFile[filepath.Base(loc.URI)]++
	}
	if byFile["Factory.php"] != 2 || byFile["Gadget.php"] != 1 {
		t.Errorf("references find the mentions composer update added to vendor files: %v", byFile)
	}

	var found []struct{ Name string }
	resp := h.readResponse(h.send("workspace/symbol", map[string]interface{}{"query": "Discontinued"}))
	raw, _ := json.Marshal(resp["result"])
	if err := json.Unmarshal(raw, &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("a class composer update removed is gone from the index: %v", found)
	}
}

func TestComposerChangesInQuickSuccessionReindexOnce(t *testing.T) {
	h, root := startComposerProject(t)
	waitForLog(t, h, "PHP LSP: Indexed 3 vendor files")

	h.notify("workspace/didChangeWatchedFiles", map[string]interface{}{"changes": "not a list"})
	for _, name := range []string{"composer.lock", "vendor/composer/installed.json"} {
		h.notify("workspace/didChangeWatchedFiles", map[string]interface{}{
			"changes": []map[string]interface{}{{"uri": symbols.PathToURI(filepath.Join(root, name)), "type": 2}},
		})
	}
	waitForLog(t, h, "PHP LSP: Re-indexed 3 vendor files after a Composer change")

	deadline := time.After(2 * watchedFilesDelay)
	for {
		select {
		case msg := <-h.notes:
			params, _ := msg["params"].(map[string]interface{})
			if text, _ := params["message"].(string); strings.HasPrefix(text, "PHP LSP: Re-indexed") {
				t.Fatalf("changes inside the delay re-index once, got a second run: %q", text)
			}
		case <-deadline:
			return
		}
	}
}
