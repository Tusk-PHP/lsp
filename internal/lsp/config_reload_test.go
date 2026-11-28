package lsp

import (
	"path/filepath"
	"slices"
	"testing"
)

const usesMissingClass = "<?php\nnamespace App;\nfunction make() { return new Missing(); }\n"

func TestDiagnosticRulesInTheConfigFileApplyWithoutARestart(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":  `{"require": {"php": "^8.1"}}`,
		".tusk-php.json": `{}`,
	}, map[string]interface{}{}, nil)
	uri := "file://" + filepath.Join(root, "app/make.php")
	openDocument(h, uri, usesMissingClass)
	waitForDiagnostic(t, h, uri, "unknown-class", true)

	writeProjectFiles(t, root, map[string]string{".tusk-php.json": `{"diagnosticRules": {"unknown-class": "off"}}`})
	notifyWatched(h, root, ".tusk-php.json")
	waitForDiagnostic(t, h, uri, "unknown-class", false)

	writeProjectFiles(t, root, map[string]string{".tusk-php.json": `{"diagnosticIgnore": [{"code": "unknown-class", "path": "lib"}]}`})
	notifyWatched(h, root, ".tusk-php.json")
	waitForDiagnostic(t, h, uri, "unknown-class", true)

	writeProjectFiles(t, root, map[string]string{".tusk-php.json": `{"diagnosticIgnore": [{"code": "unknown-class", "path": "app"}]}`})
	notifyWatched(h, root, ".tusk-php.json")
	waitForDiagnostic(t, h, uri, "unknown-class", false)
}

func TestDisablingDiagnosticsClearsThem(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{"composer.json": `{"require": {"php": "^8.1"}}`}, map[string]interface{}{}, nil)
	uri := "file://" + filepath.Join(root, "app/make.php")
	openDocument(h, uri, usesMissingClass)
	waitForDiagnostic(t, h, uri, "unknown-class", true)

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{"diagnosticsEnabled": false}})
	waitForDiagnostic(t, h, uri, "unknown-class", false)

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{"diagnosticsEnabled": true}})
	waitForDiagnostic(t, h, uri, "unknown-class", true)
}

func TestExcludePathsAndMaxIndexFilesApplyWithoutARestart(t *testing.T) {
	h, _ := startWatchedProject(t, map[string]string{
		"composer.json":        `{"require": {"php": "^8.1"}}`,
		"app/Order.php":        "<?php\nnamespace App;\nclass Order {}\n",
		"legacy/OldOrder.php":  "<?php\nnamespace Legacy;\nclass OldOrder {}\n",
		"legacy/zz/Ledger.php": "<?php\nnamespace Legacy;\nclass Ledger {}\n",
	}, map[string]interface{}{}, nil)
	waitForSymbols(t, h, "OldOrder", func(names []string) bool { return slices.Contains(names, "OldOrder") })

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{"excludePaths": []string{"vendor", "legacy"}}})
	waitForSymbols(t, h, "OldOrder", func(names []string) bool { return len(names) == 0 })
	if names := symbolNames(t, h, "Order"); !slices.Contains(names, "Order") {
		t.Errorf("files outside the new exclusion stay indexed: %v", names)
	}

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{"maxIndexFiles": 2}})
	waitForSymbols(t, h, "OldOrder", func(names []string) bool { return slices.Contains(names, "OldOrder") })
	if names := symbolNames(t, h, "Ledger"); len(names) != 0 {
		t.Errorf("files past the new maxIndexFiles leave the index: %v", names)
	}

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{"maxIndexFiles": 0}})
	waitForSymbols(t, h, "Ledger", func(names []string) bool { return slices.Contains(names, "Ledger") })
}

func TestEmptyClientSettingsKeepTheInitializationOptions(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":       `{"require": {"php": "^8.1"}}`,
		"legacy/OldOrder.php": "<?php\nnamespace Legacy;\nclass OldOrder {}\n",
	}, map[string]interface{}{}, map[string]interface{}{"diagnosticsEnabled": false, "excludePaths": []string{"vendor", "legacy"}})
	uri := "file://" + filepath.Join(root, "app/make.php")
	openDocument(h, uri, usesMissingClass)

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{}})
	settleRefreshes(t, h, root, "AfterEmptySettings")
	if cfg := h.server.config(); cfg.DiagnosticsEnabled || !slices.Contains(cfg.ExcludePaths, "legacy") {
		t.Errorf("settings without Tusk PHP keys change nothing: diagnostics %v, excludePaths %v", cfg.DiagnosticsEnabled, cfg.ExcludePaths)
	}
	if names := symbolNames(t, h, "OldOrder"); len(names) != 0 {
		t.Errorf("the excluded directory stays out of the index: %v", names)
	}

	h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": map[string]interface{}{"phpstanEnabled": false}})
	settleRefreshes(t, h, root, "AfterPHPStanSetting")
	if cfg := h.server.config(); cfg.DiagnosticsEnabled || !slices.Contains(cfg.ExcludePaths, "legacy") {
		t.Errorf("settings apply over the initialization options: diagnostics %v, excludePaths %v", cfg.DiagnosticsEnabled, cfg.ExcludePaths)
	}
}

func TestAConfigFileWithASyntaxErrorKeepsItsExclusions(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":       `{"require": {"php": "^8.1"}}`,
		".tusk-php.json":      `{"excludePaths": ["vendor", "legacy"], "diagnosticRules": {"unknown-class": "off"}}`,
		"app/Order.php":       "<?php\nnamespace App;\nclass Order {}\n",
		"legacy/OldOrder.php": "<?php\nnamespace Legacy;\nclass OldOrder {}\n",
	}, map[string]interface{}{}, nil)
	waitForSymbols(t, h, "Order", func(names []string) bool { return slices.Contains(names, "Order") })

	writeProjectFiles(t, root, map[string]string{".tusk-php.json": `{"excludePaths": ["vendor"`})
	notifyWatched(h, root, ".tusk-php.json")
	waitForShowMessage(t, h, "Tusk PHP kept its previous configuration")
	settleRefreshes(t, h, root, "AfterBrokenSave")
	if names := symbolNames(t, h, "OldOrder"); len(names) != 0 {
		t.Errorf("a half-edited config file keeps the last good exclusions: %v", names)
	}
	if h.server.config().IsRuleEnabled("unknown-class") {
		t.Error("a half-edited config file keeps the last good rules")
	}
}

func TestSettingsWithoutTuskPHPKeysQueueNoRefresh(t *testing.T) {
	h, _ := startWatchedProject(t, map[string]string{"composer.json": `{"require": {"php": "^8.1"}}`}, map[string]interface{}{}, nil)
	<-h.server.indexed
	for _, settings := range []interface{}{map[string]interface{}{}, map[string]interface{}{"editor.tabSize": 4}, nil} {
		h.notify("workspace/didChangeConfiguration", map[string]interface{}{"settings": settings})
		symbolNames(t, h, "Anything")
		w := &h.server.watch
		w.mu.Lock()
		queued := w.config
		w.mu.Unlock()
		if queued {
			t.Errorf("settings %v hold no Tusk PHP keys and queue no refresh", settings)
		}
	}
}
