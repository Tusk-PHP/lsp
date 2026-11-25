package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	directories := false
	for _, w := range registration["registerOptions"].(map[string]interface{})["watchers"].([]interface{}) {
		watcher := w.(map[string]interface{})
		globs = append(globs, watcher["globPattern"].(string))
		directories = directories || watcher["globPattern"] == "**/*" && watcher["kind"] == float64(5)
	}
	if !directories {
		t.Errorf("a create/delete watcher on **/* reports directories: %v", registration)
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

func symbolURIs(t *testing.T, h *lspHarness, query string) []string {
	t.Helper()
	var found []struct {
		Name     string
		Location struct{ URI string }
	}
	resp := h.readResponse(h.send("workspace/symbol", map[string]interface{}{"query": query}))
	raw, _ := json.Marshal(resp["result"])
	if err := json.Unmarshal(raw, &found); err != nil {
		t.Fatal(err)
	}
	uris := make([]string, 0, len(found))
	for _, f := range found {
		if f.Name == query {
			uris = append(uris, f.Location.URI)
		}
	}
	return uris
}

func notifyChanges(h *lspHarness, root string, changes map[string]int) {
	events := make([]map[string]interface{}, 0, len(changes))
	for name, kind := range changes {
		events = append(events, map[string]interface{}{"uri": "file://" + filepath.Join(root, name), "type": kind})
	}
	h.notify("workspace/didChangeWatchedFiles", map[string]interface{}{"changes": events})
}

func TestDeletedDirectoryLeavesTheIndex(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":               `{"require": {"php": "^8.1"}}`,
		"app/Billing/Invoice.php":     "<?php\nnamespace App\\Billing;\nclass Invoice {}\n",
		"app/Billing/Tax/Rate.php":    "<?php\nnamespace App\\Billing\\Tax;\nclass Rate {}\n",
		"app/BillingKept/Receipt.php": "<?php\nnamespace App\\BillingKept;\nclass Receipt {}\n",
	}, watchingClient(), nil)
	waitForSymbols(t, h, "Rate", func(names []string) bool { return slices.Contains(names, "Rate") })

	if err := os.RemoveAll(filepath.Join(root, "app/Billing")); err != nil {
		t.Fatal(err)
	}
	notifyChanges(h, root, map[string]int{"app/Billing": 3})
	waitForSymbols(t, h, "Invoice", func(names []string) bool { return len(names) == 0 })
	if names := symbolNames(t, h, "Rate"); len(names) != 0 {
		t.Errorf("classes in a deleted directory's subdirectories leave too: %v", names)
	}
	if names := symbolNames(t, h, "Receipt"); !slices.Contains(names, "Receipt") {
		t.Errorf("a sibling directory sharing the name's prefix stays: %v", names)
	}
}

func TestRenamedDirectoryResolvesAtTheNewPlace(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":         `{"require": {"php": "^8.1"}}`,
		"app/Old/Shipment.php":  "<?php\nnamespace App\\Ship;\nclass Shipment {}\n",
		"app/Old/Track/Leg.php": "<?php\nnamespace App\\Ship;\nclass Leg {}\n",
	}, watchingClient(), nil)
	waitForSymbols(t, h, "Leg", func(names []string) bool { return slices.Contains(names, "Leg") })

	if err := os.Rename(filepath.Join(root, "app/Old"), filepath.Join(root, "app/New")); err != nil {
		t.Fatal(err)
	}
	notifyChanges(h, root, map[string]int{"app/Old": 3, "app/New": 1})
	want := "file://" + filepath.Join(root, "app/New/Track/Leg.php")
	deadline := time.Now().Add(10 * time.Second)
	for !slices.Equal(symbolURIs(t, h, "Leg"), []string{want}) {
		if time.Now().After(deadline) {
			t.Fatalf("Leg is declared at the new place only: %v", symbolURIs(t, h, "Leg"))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := symbolURIs(t, h, "Shipment"); !slices.Equal(got, []string{"file://" + filepath.Join(root, "app/New/Shipment.php")}) {
		t.Errorf("Shipment is declared at the new place only: %v", got)
	}
}

func waitForShowMessage(t *testing.T, h *lspHarness, prefix string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case msg := <-h.notes:
			if msg["method"] != "window/showMessage" {
				continue
			}
			if text, _ := msg["params"].(map[string]interface{})["message"].(string); strings.HasPrefix(text, prefix) {
				return
			}
		case <-deadline:
			t.Fatalf("no window/showMessage starting with %q", prefix)
		}
	}
}

func settleRefreshes(t *testing.T, h *lspHarness, root, class string) {
	t.Helper()
	name := "app/" + class + ".php"
	writeProjectFiles(t, root, map[string]string{name: "<?php\nnamespace App;\nclass " + class + " {}\n"})
	notifyWatched(h, root, name)
	waitForSymbols(t, h, class, func(names []string) bool { return slices.Contains(names, class) })
}

func TestClientSettingsMergeOverTheInitializationOptions(t *testing.T) {
	h, _ := startWatchedProject(t, map[string]string{
		"composer.json": `{"name": "app/app"}`,
	}, map[string]interface{}{}, map[string]interface{}{"phpVersion": "7.4", "excludePaths": []string{"vendor", "legacy"}})

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{}})
	symbolNames(t, h, "Anything")
	if cfg := h.server.loadConfig(); cfg.PHPVersion != "7.4" || !cfg.IsPHPVersionExplicitlySet() {
		t.Errorf("settings with no Tusk PHP keys leave the initialization options in place: PHP %s", cfg.PHPVersion)
	}

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{"diagnosticsEnabled": false}})
	symbolNames(t, h, "Anything")
	cfg := h.server.loadConfig()
	if cfg.PHPVersion != "7.4" || !slices.Equal(cfg.ExcludePaths, []string{"vendor", "legacy"}) {
		t.Errorf("settings apply over the initialization options instead of replacing them: PHP %s, excludePaths %v", cfg.PHPVersion, cfg.ExcludePaths)
	}
	if cfg.DiagnosticsEnabled {
		t.Error("the settings' own keys apply")
	}

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{"phpVersion": "8.4"}})
	symbolNames(t, h, "Anything")
	if cfg := h.server.loadConfig(); cfg.PHPVersion != "8.4" || !cfg.DiagnosticsEnabled {
		t.Errorf("newer settings replace older settings: PHP %s, diagnostics %v", cfg.PHPVersion, cfg.DiagnosticsEnabled)
	}
}

func TestTuskSettingsIgnoresObjectsWithoutTuskKeys(t *testing.T) {
	for raw, want := range map[string]bool{
		``:                              false,
		`null`:                          false,
		`{}`:                            false,
		`{"otherServer": {"a": 1}}`:     false,
		`{"phpVersion": "8.2"}`:         true,
		`{"diagnosticsEnabled": false}`: true,
	} {
		got, err := tuskSettings(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if (got != nil) != want {
			t.Errorf("%s: settings applied = %v, want %v", raw, got != nil, want)
		}
	}
	if _, err := tuskSettings(json.RawMessage(`{"phpVersion": 8}`)); err == nil {
		t.Error("a Tusk PHP key of the wrong type is an error")
	}
}

func TestAConfigFileWithASyntaxErrorKeepsTheLastGoodOne(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":  `{"name": "app/app"}`,
		".tusk-php.json": `{"phpVersion": "7.4"}`,
	}, map[string]interface{}{}, nil)
	uri := "file://" + filepath.Join(root, "check.php")
	openDocument(h, uri, usesPHP80)
	waitForDiagnostic(t, h, uri, "builtin-unavailable", true)

	writeProjectFiles(t, root, map[string]string{".tusk-php.json": `{"phpVersion": "8.4"`})
	notifyWatched(h, root, ".tusk-php.json")
	waitForShowMessage(t, h, "Tusk PHP kept its previous configuration")
	settleRefreshes(t, h, root, "AfterBrokenSave")
	if version := h.server.workspace.Profile().PHPVersion; version != "7.4" {
		t.Errorf("a half-edited config file keeps the last good PHP version, got %s", version)
	}

	writeProjectFiles(t, root, map[string]string{".tusk-php.json": `{"phpVersion": "8.4"}`})
	notifyWatched(h, root, ".tusk-php.json")
	waitForDiagnostic(t, h, uri, "builtin-unavailable", false)
}
