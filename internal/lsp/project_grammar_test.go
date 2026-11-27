package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const php74Source = `<?php
#[ a comment before PHP 8
function match($value) {
    return $value;
}
class After {}
echo match(1);
`

func TestTheProjectsPHPVersionDecidesTheGrammar(t *testing.T) {
	cases := []struct {
		name, constraint string
		syntaxError      bool
	}{
		{"a PHP 7.4 project reads #[ as a comment and match as a name", "^7.4", false},
		{"a PHP 8.0 project reads #[ as an attribute", "^8.0", true},
		{"a project that also runs on PHP 7.4 reads a file that only parses on 7.4 as 7.4", ">=7.4", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "case.php")
			if err := os.WriteFile(path, []byte(php74Source), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require": {"php": "`+tc.constraint+`"}}`), 0o644); err != nil {
				t.Fatal(err)
			}
			uri := "file://" + path
			h := newHarness(t)
			defer h.close()
			h.readResponse(h.send("initialize", map[string]interface{}{
				"rootUri":      "file://" + root,
				"capabilities": map[string]interface{}{},
			}))
			h.notify("initialized", map[string]interface{}{})
			h.notify("textDocument/didOpen", map[string]interface{}{
				"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": php74Source},
			})
			if diags := collectDiagnostics(h, uri); hasDiagnostic(diags, "syntax-error") != tc.syntaxError {
				t.Errorf("syntax error = %v, want %v: %v", !tc.syntaxError, tc.syntaxError, diags)
			}
			if tc.syntaxError {
				return
			}
			resp := h.readResponse(h.send("textDocument/definition", map[string]interface{}{
				"textDocument": map[string]interface{}{"uri": uri},
				"position":     map[string]interface{}{"line": 6, "character": 6},
			}))
			if got := responseText(resp); !strings.Contains(got, `"line":2`) || !strings.Contains(got, uri) {
				t.Errorf("definition of match(1) = %s, want line 2 of %s", got, uri)
			}
		})
	}
}

func TestAProjectThatAlsoRunsOnPHP8KeepsItsAttributes(t *testing.T) {
	src := `<?php
#[AllowDynamicProperties]
class Skin {
    public function feedback(): void {}
}
function run(Skin $skin): void {
    $skin->overwrite = true;
}
`
	root := t.TempDir()
	path := filepath.Join(root, "case.php")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require": {"php": ">=7.4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := "file://" + path
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{
		"rootUri":      "file://" + root,
		"capabilities": map[string]interface{}{},
	}))
	h.notify("initialized", map[string]interface{}{})
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": src},
	})
	if diags := collectDiagnostics(h, uri); hasDiagnostic(diags, "unknown-member") || hasDiagnostic(diags, "syntax-error") {
		t.Errorf("want no unknown-member or syntax-error, got %v", diags)
	}
}
