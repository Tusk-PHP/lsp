package workspace

import (
	"io"
	"log"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestValidationExtensionsFollowIndexing(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"composer.json":           `{"autoload": {"psr-4": {"App\\": "app/"}}}`,
		"bootstrap/app.php":       "<?php\nIlluminate\\Support\\Facades\\Validator::extend('slug', fn () => true);\n",
		"app/Rules/Registers.php": "<?php\nnamespace App\\Rules;\nuse Illuminate\\Support\\Facades\\Validator;\nValidator::extendImplicit('isbn', fn () => true);\n",
	})
	ws := New(Options{RootPath: root, Framework: "laravel", Config: config.DefaultConfig(), Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	ws.IndexWorkspace()
	rules := frameworklaravel.NewValidationRules(ws.Index, ws.ValidationExtensions, ws.ValidationHelpers, func(string) string { return "" })
	for _, name := range []string{"slug", "isbn"} {
		if _, ok := rules.Lookup(name); !ok {
			t.Errorf("%s is registered outside the providers", name)
		}
	}

	uri := symbols.PathToURI(filepath.Join(root, "app/Rules/Registers.php"))
	ws.IndexFile(uri, "<?php\nnamespace App\\Rules;\nuse Illuminate\\Support\\Facades\\Validator;\nValidator::extend('ean', fn () => true);\n")
	if _, ok := rules.Lookup("isbn"); ok {
		t.Error("isbn is gone once its file no longer registers it")
	}
	if _, ok := rules.Lookup("ean"); !ok {
		t.Error("ean comes with the edit")
	}
	ws.RemoveFile(uri)
	if _, ok := rules.Lookup("ean"); ok {
		t.Error("a removed file registers nothing")
	}
}
