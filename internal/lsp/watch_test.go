package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func writeProjectFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func startWatchedProject(t *testing.T, files map[string]string, capabilities, options map[string]interface{}) (*lspHarness, string) {
	t.Helper()
	root := t.TempDir()
	writeProjectFiles(t, root, files)
	h := newHarness(t)
	t.Cleanup(h.close)
	params := map[string]interface{}{"rootUri": "file://" + root, "processId": nil, "capabilities": capabilities}
	if options != nil {
		params["initializationOptions"] = options
	}
	h.readResponse(h.send("initialize", params))
	h.notify("initialized", map[string]interface{}{})
	return h, root
}

func watchingClient() map[string]interface{} {
	return map[string]interface{}{
		"workspace": map[string]interface{}{
			"didChangeWatchedFiles": map[string]interface{}{"dynamicRegistration": true},
		},
	}
}

func symbolNames(t *testing.T, h *lspHarness, query string) []string {
	t.Helper()
	var found []struct{ Name string }
	resp := h.readResponse(h.send("workspace/symbol", map[string]interface{}{"query": query}))
	raw, _ := json.Marshal(resp["result"])
	if err := json.Unmarshal(raw, &found); err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(found))
	for i, f := range found {
		names[i] = f.Name
	}
	return names
}

func waitForSymbols(t *testing.T, h *lspHarness, query string, want func([]string) bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		names := symbolNames(t, h, query)
		if want(names) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("workspace/symbol %q kept answering %v", query, names)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForDiagnostic(t *testing.T, h *lspHarness, uri, code string, present bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case msg := <-h.notes:
			if msg["method"] != "textDocument/publishDiagnostics" {
				continue
			}
			params := msg["params"].(map[string]interface{})
			if params["uri"] != uri {
				continue
			}
			found := false
			for _, d := range params["diagnostics"].([]interface{}) {
				if d.(map[string]interface{})["code"] == code {
					found = true
				}
			}
			if found == present {
				return
			}
		case <-deadline:
			t.Fatalf("no diagnostics for %s with %s present=%v", uri, code, present)
		}
	}
}

func openDocument(h *lspHarness, uri, text string) {
	h.notify("textDocument/didOpen", map[string]interface{}{"textDocument": map[string]interface{}{
		"uri": uri, "languageId": "php", "version": 1, "text": text,
	}})
}

func notifyWatched(h *lspHarness, root string, names ...string) {
	changes := make([]map[string]interface{}, len(names))
	for i, name := range names {
		changes[i] = map[string]interface{}{"uri": "file://" + filepath.Join(root, name), "type": 2}
	}
	h.notify("workspace/didChangeWatchedFiles", map[string]interface{}{"changes": changes})
}

func TestWatchedFilesAreRegisteredWithClientsThatCanRegisterWatchers(t *testing.T) {
	files := map[string]string{"composer.json": `{"require": {"php": "^8.1"}}`}
	h, _ := startWatchedProject(t, files, watchingClient(), nil)
	var req map[string]interface{}
	select {
	case req = <-h.responses:
	case <-time.After(5 * time.Second):
		t.Fatal("a client that registers watchers is asked to watch files")
	}
	if req["method"] != "client/registerCapability" {
		t.Fatalf("expected client/registerCapability, got %v", req)
	}
	registration := req["params"].(map[string]interface{})["registrations"].([]interface{})[0].(map[string]interface{})
	var globs []string
	for _, w := range registration["registerOptions"].(map[string]interface{})["watchers"].([]interface{}) {
		globs = append(globs, w.(map[string]interface{})["globPattern"].(string))
	}
	for _, want := range []string{"**/*.php", "**/*.phtml", "**/composer.json", "**/composer.lock", "**/vendor/composer/installed.json", "**/.tusk-php.json"} {
		if !slices.Contains(globs, want) {
			t.Errorf("the watcher covers %s: %v", want, globs)
		}
	}

	h, _ = startWatchedProject(t, files, map[string]interface{}{}, nil)
	select {
	case req := <-h.responses:
		t.Errorf("a client that can't register watchers is sent no request: %v", req)
	case <-time.After(500 * time.Millisecond):
	}
}

func TestFilesChangedOutsideTheEditorReachTheIndex(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":    `{"require": {"php": "^8.1"}}`,
		"app/Discount.php": "<?php\nnamespace App;\nclass Discount {}\n",
	}, map[string]interface{}{}, nil)
	waitForSymbols(t, h, "Discount", func(names []string) bool { return slices.Contains(names, "Discount") })

	user := "file://" + filepath.Join(root, "app/Checkout.php")
	openDocument(h, user, "<?php\nnamespace App;\nfunction checkout() { return new Discount(); }\n")
	waitForDiagnostic(t, h, user, "unknown-class", false)

	if err := os.Remove(filepath.Join(root, "app/Discount.php")); err != nil {
		t.Fatal(err)
	}
	writeProjectFiles(t, root, map[string]string{
		"app/Coupon.php":          "<?php\nnamespace App;\nclass Coupon {}\n",
		"vendor/acme/Ignored.php": "<?php\nclass IgnoredVendorClass {}\n",
		"node_modules/x/Skip.php": "<?php\nclass SkippedNodeClass {}\n",
	})
	notifyWatched(h, root, "app/Discount.php", "app/Coupon.php", "vendor/acme/Ignored.php", "node_modules/x/Skip.php")

	waitForDiagnostic(t, h, user, "unknown-class", true)
	waitForSymbols(t, h, "Coupon", func(names []string) bool { return slices.Contains(names, "Coupon") })
	if names := symbolNames(t, h, "Discount"); slices.Contains(names, "Discount") {
		t.Errorf("a deleted file's class leaves the index: %v", names)
	}
	for _, excluded := range []string{"IgnoredVendorClass", "SkippedNodeClass"} {
		if names := symbolNames(t, h, excluded); len(names) != 0 {
			t.Errorf("files under excludePaths stay out of the index: %v", names)
		}
	}
}

func TestOpenDocumentsKeepTheEditorsText(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":   `{"require": {"php": "^8.1"}}`,
		"app/Invoice.php": "<?php\nnamespace App;\nclass Invoice {}\n",
	}, map[string]interface{}{}, nil)
	waitForSymbols(t, h, "Invoice", func(names []string) bool { return slices.Contains(names, "Invoice") })

	uri := "file://" + filepath.Join(root, "app/Invoice.php")
	openDocument(h, uri, "<?php\nnamespace App;\nclass Invoice {}\nclass EditedInEditor {}\n")
	writeProjectFiles(t, root, map[string]string{
		"app/Invoice.php": "<?php\nnamespace App;\nclass Invoice {}\nclass WrittenOnDisk {}\n",
		"app/Probe.php":   "<?php\nnamespace App;\nclass Probe {}\n",
	})
	notifyWatched(h, root, "app/Invoice.php", "app/Probe.php")
	waitForSymbols(t, h, "Probe", func(names []string) bool { return slices.Contains(names, "Probe") })
	if names := symbolNames(t, h, "WrittenOnDisk"); len(names) != 0 {
		t.Errorf("the editor's text of an open document wins over the disk: %v", names)
	}
	if names := symbolNames(t, h, "EditedInEditor"); len(names) != 1 {
		t.Errorf("the editor's text stays indexed: %v", names)
	}
}

const usesPHP80 = "<?php\nfunction check(string $s): bool { return str_contains($s, 'x'); }\n"

func TestComposerPHPVersionChangeAppliesWithoutARestart(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json": `{"require": {"php": "^8.1"}}`,
	}, map[string]interface{}{}, nil)
	uri := "file://" + filepath.Join(root, "check.php")
	openDocument(h, uri, usesPHP80)
	waitForDiagnostic(t, h, uri, "builtin-unavailable", false)

	writeProjectFiles(t, root, map[string]string{"composer.json": `{"require": {"php": "^7.4"}}`})
	notifyWatched(h, root, "composer.json")
	waitForDiagnostic(t, h, uri, "builtin-unavailable", true)

	writeProjectFiles(t, root, map[string]string{"composer.json": `{"require": {"php": "^8.2"}}`})
	notifyWatched(h, root, "composer.json")
	waitForDiagnostic(t, h, uri, "builtin-unavailable", false)
}

func TestConfigFileAndClientSettingsChangeThePHPVersion(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":  `{"name": "app/app"}`,
		".tusk-php.json": `{"phpVersion": "8.3"}`,
	}, map[string]interface{}{}, nil)
	uri := "file://" + filepath.Join(root, "check.php")
	openDocument(h, uri, usesPHP80)
	waitForDiagnostic(t, h, uri, "builtin-unavailable", false)

	writeProjectFiles(t, root, map[string]string{".tusk-php.json": `{"phpVersion": "7.4"}`})
	notifyWatched(h, root, ".tusk-php.json")
	waitForDiagnostic(t, h, uri, "builtin-unavailable", true)

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{"phpVersion": "8.4"}})
	waitForDiagnostic(t, h, uri, "builtin-unavailable", false)
}
