package models

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func variableKeysAt(r *FrameworkArrayResolver, idx *symbols.Index, marked string) []string {
	before, after, _ := strings.Cut(marked, "‸")
	src := before + after
	off := uint32(len(before))
	file := parser.ParseFile(src, token.Latest)
	var v *ast.Variable
	ast.Walk(file.Tree, func(n ast.Node) bool {
		if found, ok := n.(*ast.Variable); ok && found.Start == off {
			v = found
		}
		return true
	})
	if v == nil {
		return nil
	}
	var keys []string
	for _, f := range r.VariableKeys(resolve.NewResolver(idx), file, v) {
		keys = append(keys, f.Key)
	}
	return keys
}

func TestVariableKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "app.php"), []byte("<?php\nreturn ['name' => 'x', 'db' => ['host' => 'h']];\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(dir, "StoreRequest.php")
	request := "<?php\nnamespace App;\nclass StoreRequest {\n    public function rules(): array {\n        return ['title' => 'required'];\n    }\n    public function validated(): array { return []; }\n}\n"
	if err := os.WriteFile(requestPath, []byte(request), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFileWithSource("file://"+requestPath, request, symbols.SourceProject)
	laravel := NewFrameworkArrayResolver(idx, dir, "laravel")
	other := NewFrameworkArrayResolver(idx, dir, "none")

	tests := []struct {
		name     string
		resolver *FrameworkArrayResolver
		code     string
		want     []string
	}{
		{"a literal's shape", nil, "<?php\nfunction f() {\n    $d = ['name' => 1, 'age' => 2];\n    ‸$d[''];\n}\n", []string{"name", "age"}},
		{"a config file", laravel, "<?php\nfunction f() {\n    $c = config('app');\n    ‸$c[''];\n}\n", []string{"name", "db"}},
		{"a config path", laravel, "<?php\nfunction f() {\n    $c = config('app.db');\n    ‸$c[''];\n}\n", []string{"host"}},
		{"every config file", laravel, "<?php\nfunction f() {\n    $c = config();\n    ‸$c[''];\n}\n", []string{"app"}},
		{"a dynamic config path", laravel, "<?php\nfunction f($p) {\n    $c = config($p);\n    ‸$c[''];\n}\n", nil},
		{"another function", laravel, "<?php\nfunction f() {\n    $c = strtolower('app');\n    ‸$c[''];\n}\n", nil},
		{"config outside Laravel", other, "<?php\nfunction f() {\n    $c = config('app');\n    ‸$c[''];\n}\n", nil},
		{"config without a resolver", nil, "<?php\nfunction f() {\n    $c = config('app');\n    ‸$c[''];\n}\n", nil},
		{"a method's keys", laravel, "<?php\nfunction f(\\App\\StoreRequest $r) {\n    $d = $r->validated();\n    ‸$d[''];\n}\n", []string{"title"}},
		{"another value", laravel, "<?php\nfunction f($x) {\n    $d = $x;\n    ‸$d[''];\n}\n", nil},
		{
			"keys written one by one",
			nil,
			"<?php\nfunction f() {\n    $r = [];\n    $r['a'] = 1;\n    $r['a'] = 2;\n    $r['b'] .= 'x';\n    $r[0] = 1;\n    $r = 3;\n    $o['c'] = 1;\n    $g = function () { $r['inner'] = 1; };\n    ‸$r[''];\n    $r['later'] = 1;\n}\n",
			[]string{"a", "later"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := variableKeysAt(tt.resolver, idx, tt.code); !slices.Equal(got, tt.want) {
				t.Errorf("keys = %v, want %v", got, tt.want)
			}
		})
	}
}
