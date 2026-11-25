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
