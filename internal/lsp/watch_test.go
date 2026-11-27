package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/symbols"
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
	return startWatchedProjectIn(t, t.TempDir(), files, capabilities, options)
}

func startWatchedProjectIn(t *testing.T, root string, files map[string]string, capabilities, options map[string]interface{}) (*lspHarness, string) {
	t.Helper()
	writeProjectFiles(t, root, files)
	h := newHarness(t)
	t.Cleanup(h.close)
	params := map[string]interface{}{"rootUri": symbols.PathToURI(root), "processId": nil, "capabilities": capabilities}
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
		changes[i] = map[string]interface{}{"uri": symbols.PathToURI(filepath.Join(root, name)), "type": 2}
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
	for _, want := range []string{"**/*.php", "**/*.phtml", "**/composer.json", "**/composer.lock", "**/vendor/composer/installed.json", "**/.tusk-php.json", "**/lang/*.json"} {
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

	h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})
	waitForSymbols(t, h, "WrittenOnDisk", func(names []string) bool { return len(names) == 1 })
	if names := symbolNames(t, h, "EditedInEditor"); len(names) != 0 {
		t.Errorf("closing a document without saving gives the disk its text back: %v", names)
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
		events = append(events, map[string]interface{}{"uri": symbols.PathToURI(filepath.Join(root, name)), "type": kind})
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
	<-h.server.indexed
	name := "app/" + class + ".php"
	writeProjectFiles(t, root, map[string]string{name: "<?php\nnamespace App;\nclass " + class + " {}\n"})
	notifyWatched(h, root, name)
	waitForSymbols(t, h, class, func(names []string) bool { return slices.Contains(names, class) })
	h.server.watch.refresh.Lock()
	h.server.watch.refresh.Unlock()
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

func TestADocumentOpenedWhileItsDiskTextIsIndexedGetsTheEditorsTextBack(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":   `{"require": {"php": "^8.1"}}`,
		"app/Invoice.php": "<?php\nnamespace App;\nclass Invoice {}\nclass WrittenOnDisk {}\n",
	}, map[string]interface{}{}, nil)
	waitForSymbols(t, h, "WrittenOnDisk", func(names []string) bool { return len(names) == 1 })

	uri := symbols.PathToURI(filepath.Join(root, "app/Invoice.php"))
	openDocument(h, uri, "<?php\nnamespace App;\nclass Invoice {}\nclass EditedInEditor {}\n")
	waitForSymbols(t, h, "EditedInEditor", func(names []string) bool { return len(names) == 1 })
	h.server.workspace.IndexFile(uri, "<?php\nnamespace App;\nclass Invoice {}\nclass WrittenOnDisk {}\n")
	h.server.handBackToEditor([]string{uri, symbols.PathToURI(filepath.Join(root, "app/Closed.php"))})
	waitForSymbols(t, h, "EditedInEditor", func(names []string) bool { return len(names) == 1 })
	if names := symbolNames(t, h, "WrittenOnDisk"); len(names) != 0 {
		t.Errorf("the disk text a refresh indexed under an open document is replaced: %v", names)
	}
}

func TestMalformedOrEmptyChangeNotificationsChangeNothing(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json": `{"name": "app/app"}`,
	}, map[string]interface{}{}, map[string]interface{}{"phpVersion": "8.2"})
	<-h.server.indexed
	writeProjectFiles(t, root, map[string]string{"app/Unseen.php": "<?php\nnamespace App;\nclass Unseen {}\n"})

	h.notify("workspace/didChangeWatchedFiles", map[string]interface{}{"changes": "app/Unseen.php"})
	h.notify("workspace/didChangeWatchedFiles", map[string]interface{}{"changes": []interface{}{}})
	h.notify("workspace/didChangeConfiguration", "phpVersion=7.4")
	time.Sleep(3 * watchedFilesDelay)

	if names := symbolNames(t, h, "Unseen"); len(names) != 0 {
		t.Errorf("no valid notification named the new file, so it isn't indexed yet: %v", names)
	}
	if version := h.server.loadConfig().PHPVersion; version != "8.2" {
		t.Errorf("malformed settings leave the configuration alone, got PHP %s", version)
	}
	notifyWatched(h, root, "app/Unseen.php")
	waitForSymbols(t, h, "Unseen", func(names []string) bool { return slices.Contains(names, "Unseen") })
}

func TestSettingsOfTheWrongTypeAreReportedAndIgnored(t *testing.T) {
	h, _ := startWatchedProject(t, map[string]string{
		"composer.json": `{"name": "app/app"}`,
	}, map[string]interface{}{}, map[string]interface{}{"phpVersion": "8.2"})

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{"phpVersion": 7.4}})
	waitForShowMessage(t, h, "Tusk PHP ignored the new settings")
	if version := h.server.loadConfig().PHPVersion; version != "8.2" {
		t.Errorf("settings that can't be read leave the previous ones in place, got PHP %s", version)
	}
}

func TestAFileThatCannotBeReadIsReportedAndTheRestIndexed(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions don't stop this user from reading the file")
	}
	h, root := startWatchedProject(t, map[string]string{
		"composer.json": `{"name": "app/app"}`,
	}, map[string]interface{}{}, nil)
	writeProjectFiles(t, root, map[string]string{
		"app/Locked.php":   "<?php\nnamespace App;\nclass Locked {}\n",
		"app/Readable.php": "<?php\nnamespace App;\nclass Readable {}\n",
	})
	locked := filepath.Join(root, "app/Locked.php")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })

	notifyWatched(h, root, "app/Locked.php", "app/Readable.php")
	deadline := time.After(10 * time.Second)
	for {
		select {
		case msg := <-h.notes:
			params, _ := msg["params"].(map[string]interface{})
			text, _ := params["message"].(string)
			if msg["method"] != "window/logMessage" || !strings.HasPrefix(text, "PHP LSP: some changed files could not be re-indexed") {
				continue
			}
			if !strings.Contains(text, locked) {
				t.Errorf("the warning names the file: %q", text)
			}
			waitForSymbols(t, h, "Readable", func(names []string) bool { return slices.Contains(names, "Readable") })
			return
		case <-deadline:
			t.Fatal("no warning about the file that could not be read")
		}
	}
}

func TestANewAnonymousComponentIsOfferedAfterItIsWrittenOnDisk(t *testing.T) {
	h, root, uri := componentsWorkspace(t, "<x-\n")
	defer h.close()
	names := func() string {
		var items []testCompletionItem
		request(t, h, "textDocument/completion", uri, map[string]interface{}{"line": 0, "character": 3}, nil, &items)
		labels := make([]string, len(items))
		for i, it := range items {
			labels[i] = it.Label
		}
		return strings.Join(labels, " ")
	}
	if got := names(); got != "alert card" {
		t.Fatalf("component names before: %q", got)
	}

	writeProjectFiles(t, root, map[string]string{"resources/views/components/badge.blade.php": "<span>{{ $slot }}</span>\n"})
	notifyWatched(h, root, "resources/views/components/badge.blade.php")
	deadline := time.Now().Add(10 * time.Second)
	for names() != "alert badge card" {
		if time.Now().After(deadline) {
			t.Fatalf("the new component is offered: %q", names())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAnOpenDocumentUnderAPathWithASpaceKeepsTheEditorsText(t *testing.T) {
	for _, tc := range []struct {
		name      string
		editorURI func(path string) string
	}{
		{"the editor percent-encodes the space", symbols.PathToURI},
		{"the editor spells the URI differently from the workspace", func(path string) string { return "file://" + path }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, root := startWatchedProjectIn(t, filepath.Join(t.TempDir(), "my app"), map[string]string{
				"composer.json":   `{"require": {"php": "^8.1"}}`,
				"app/Invoice.php": "<?php\nnamespace App;\nclass Invoice {}\n",
			}, map[string]interface{}{}, nil)
			<-h.server.indexed
			path := filepath.Join(root, "app", "Invoice.php")
			uri := tc.editorURI(path)
			openDocument(h, uri, "<?php\nnamespace App;\nclass Invoice {}\nclass EditedInEditor {}\n")
			waitForSymbols(t, h, "EditedInEditor", func(names []string) bool { return len(names) == 1 })

			writeProjectFiles(t, root, map[string]string{"app/Invoice.php": "<?php\nnamespace App;\nclass Invoice {}\nclass WrittenOnDisk {}\n"})
			notifyWatched(h, root, "app/Invoice.php")
			settleRefreshes(t, h, root, "Probe")
			if names := symbolNames(t, h, "WrittenOnDisk"); len(names) != 0 {
				t.Errorf("the disk text of an open document isn't indexed next to the editor's: %v", names)
			}

			h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})
			waitForSymbols(t, h, "WrittenOnDisk", func(names []string) bool { return len(names) == 1 })
			if names := symbolNames(t, h, "EditedInEditor"); len(names) != 0 {
				t.Errorf("closing without saving drops the unsaved class: %v", names)
			}
			if got := symbolURIs(t, h, "Invoice"); !slices.Equal(got, []string{symbols.PathToURI(path)}) {
				t.Errorf("Invoice is declared once, under the workspace's URI: %v", got)
			}
		})
	}
}

func diagnosticsPublishedFor(h *lspHarness, uri string, quiet time.Duration) int {
	published := 0
	for {
		select {
		case msg := <-h.notes:
			if params, _ := msg["params"].(map[string]interface{}); msg["method"] == "textDocument/publishDiagnostics" && params["uri"] == uri {
				published++
			}
		case <-time.After(quiet):
			return published
		}
	}
}

func TestClosingADocumentReadsTheDiskBackOnlyWhenItHadUnsavedChanges(t *testing.T) {
	const invoice = "<?php\nnamespace App;\nclass Invoice {}\n"
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":    `{"require": {"php": "^8.1"}}`,
		"app/Invoice.php":  invoice,
		"app/Checkout.php": "<?php\nnamespace App;\nfunction checkout() { return new Invoice(); }\n",
	}, map[string]interface{}{}, nil)
	<-h.server.indexed
	other := symbols.PathToURI(filepath.Join(root, "app", "Checkout.php"))
	openDocument(h, other, "<?php\nnamespace App;\nfunction checkout() { return new Invoice(); }\n")
	diagnosticsPublishedFor(h, other, 3*watchedFilesDelay)

	uri := symbols.PathToURI(filepath.Join(root, "app", "Invoice.php"))
	close := func() {
		h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})
	}
	openDocument(h, uri, invoice)
	close()
	if n := diagnosticsPublishedFor(h, other, 3*watchedFilesDelay); n != 0 {
		t.Errorf("closing a document with no unsaved changes re-diagnoses the open documents %d times", n)
	}

	openDocument(h, uri, invoice)
	h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument":   map[string]interface{}{"uri": uri, "version": 2},
		"contentChanges": []map[string]interface{}{{"text": invoice + "class Draft {}\n"}},
	})
	h.notify("textDocument/didSave", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})
	close()
	if n := diagnosticsPublishedFor(h, other, 3*watchedFilesDelay); n != 0 {
		t.Errorf("closing a document right after saving it re-diagnoses the open documents %d times", n)
	}

	openDocument(h, uri, invoice)
	h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument":   map[string]interface{}{"uri": uri, "version": 2},
		"contentChanges": []map[string]interface{}{{"text": invoice + "class Draft {}\n"}},
	})
	close()
	waitForSymbols(t, h, "Draft", func(names []string) bool { return len(names) == 0 })
	if n := diagnosticsPublishedFor(h, other, 3*watchedFilesDelay); n == 0 {
		t.Error("closing a document with unsaved changes re-diagnoses the open documents against the disk text")
	}
}

func TestClosingADocumentWhileItIsIndexedInTheBackgroundLetsTheDiskTextLandLast(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json": `{"require": {"php": "^8.1"}}`,
		"app/Big.php":   "<?php\nnamespace App;\nclass OnDisk {}\n",
	}, map[string]interface{}{}, nil)
	<-h.server.indexed
	uri := symbols.PathToURI(filepath.Join(root, "app", "Big.php"))
	inFlight := func(uri, source string) {
		q := &h.server.docIndex
		q.mu.Lock()
		q.queueLocked(uri, source)
		q.mu.Unlock()
	}
	closeDoc := func(uri string) {
		h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})
	}

	openDocument(h, uri, "<?php\nnamespace App;\nclass InEditor {}\n")
	waitForSymbols(t, h, "InEditor", func(names []string) bool { return len(names) == 1 })
	inFlight(uri, "<?php\nnamespace App;\nclass LaterEdit {}\n")
	closeDoc(uri)
	time.Sleep(3 * watchedFilesDelay)
	if names := symbolNames(t, h, "OnDisk"); len(names) != 0 {
		t.Errorf("the disk text waits for the background index that is still running: %v", names)
	}
	h.server.drainDocIndex(uri)
	waitForSymbols(t, h, "OnDisk", func(names []string) bool { return len(names) == 1 })
	for _, gone := range []string{"InEditor", "LaterEdit"} {
		if names := symbolNames(t, h, gone); len(names) != 0 {
			t.Errorf("an edit waiting to be indexed when the document closed doesn't land: %s %v", gone, names)
		}
	}

	alias := "file://" + filepath.Join(root, "app") + "/./Big.php"
	reopened := "<?php\nnamespace App;\nclass Reopened {}\n"
	openDocument(h, alias, reopened)
	inFlight(alias, reopened)
	closeDoc(alias)
	openDocument(h, alias, reopened)
	symbolNames(t, h, "Reopened")
	h.server.drainDocIndex(alias)
	time.Sleep(3 * watchedFilesDelay)
	if names := symbolNames(t, h, "Reopened"); len(names) != 1 {
		t.Errorf("a document opened again before its background index finished keeps the editor's text: %v", names)
	}
}

func showMessagesStarting(h *lspHarness, prefix string, quiet time.Duration) int {
	shown := 0
	for {
		select {
		case msg := <-h.notes:
			if params, _ := msg["params"].(map[string]interface{}); msg["method"] == "window/showMessage" {
				if text, _ := params["message"].(string); strings.HasPrefix(text, prefix) {
					shown++
				}
			}
		case <-time.After(quiet):
			return shown
		}
	}
}

func TestAConfigurationProblemIsShownOnceUntilItChanges(t *testing.T) {
	const broken = `{"diagnosticIgnore": [{"message": "("}]}`
	const skipped = "Tusk PHP skipped part of its configuration"
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":  `{"name": "app/app"}`,
		".tusk-php.json": broken,
	}, map[string]interface{}{}, nil)
	settleRefreshes(t, h, root, "Started")
	if n := showMessagesStarting(h, skipped, watchedFilesDelay); n != 1 {
		t.Fatalf("the problem is shown once at startup, got %d", n)
	}

	for i, settings := range []map[string]interface{}{{"phpVersion": "8.2"}, {"phpVersion": "8.3"}} {
		h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": settings})
		settleRefreshes(t, h, root, fmt.Sprintf("AfterSettings%d", i))
	}
	notifyWatched(h, root, ".tusk-php.json")
	settleRefreshes(t, h, root, "AfterTouch")
	if n := showMessagesStarting(h, skipped, watchedFilesDelay); n != 0 {
		t.Errorf("re-reading the same problem doesn't show it again, got %d", n)
	}

	writeProjectFiles(t, root, map[string]string{".tusk-php.json": `{}`})
	notifyWatched(h, root, ".tusk-php.json")
	settleRefreshes(t, h, root, "AfterFix")
	writeProjectFiles(t, root, map[string]string{".tusk-php.json": broken})
	notifyWatched(h, root, ".tusk-php.json")
	settleRefreshes(t, h, root, "AfterBreakingAgain")
	if n := showMessagesStarting(h, skipped, watchedFilesDelay); n != 1 {
		t.Errorf("a problem that comes back after being fixed is shown again, got %d", n)
	}
}

func TestClosingADocumentTheEditorSavedWithItsTextKeepsThatText(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":    `{"require": {"php": "^8.1"}}`,
		"app/Invoice.php":  "<?php\nnamespace App;\nclass Invoice {}\n",
		"app/Checkout.php": "<?php\nnamespace App;\nfunction checkout() { return new Invoice(); }\n",
	}, map[string]interface{}{}, nil)
	<-h.server.indexed
	other := symbols.PathToURI(filepath.Join(root, "app", "Checkout.php"))
	openDocument(h, other, "<?php\nnamespace App;\nfunction checkout() { return new Invoice(); }\n")
	diagnosticsPublishedFor(h, other, 3*watchedFilesDelay)

	uri := symbols.PathToURI(filepath.Join(root, "app", "Invoice.php"))
	saved := "<?php\nnamespace App;\nclass Invoice {}\nclass Saved {}\n"
	openDocument(h, uri, "<?php\nnamespace App;\nclass Invoice {}\n")
	h.notify("textDocument/didSave", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}, "text": saved})
	waitForSymbols(t, h, "Saved", func(names []string) bool { return len(names) == 1 })
	h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})

	manifest := symbols.PathToURI(filepath.Join(root, "composer.json"))
	h.notify("textDocument/didOpen", map[string]interface{}{"textDocument": map[string]interface{}{"uri": manifest, "languageId": "json", "version": 1, "text": `{"require": {"php": "^8.2"}}`}})
	h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": manifest}})

	if n := diagnosticsPublishedFor(h, other, 3*watchedFilesDelay); n != 0 {
		t.Errorf("closing a document saved with its text, or one that isn't PHP, re-diagnoses the open documents %d times", n)
	}
	if names := symbolNames(t, h, "Saved"); len(names) != 1 {
		t.Errorf("the text the editor saved stays indexed: %v", names)
	}
}
