package completion

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestArrayKeysAreTypedFromTheTree(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///lib.php", "<?php\nclass Svc {\n    /** @return array{x: int, y: int} */\n    public function point(): array { return []; }\n}\n")
	p := NewProvider(idx, nil, "none")
	tests := []struct {
		name      string
		code      string
		want, not []string
	}{
		{
			name: "a literal assigned on the next line",
			code: "<?php\nfunction f() {\n    $data =\n        ['name' => 1, 'age' => 2];\n    $data['‸\n}\n",
			want: []string{"name", "age"},
		},
		{
			name: "a commented-out item",
			code: "<?php\nfunction f() {\n    $data = [\n        'name' => 1, // 'fake' => 2,\n        /* 'other' => 3, */ 'age' => 4,\n    ];\n    $data['‸\n}\n",
			want: []string{"name", "age"},
			not:  []string{"fake", "other"},
		},
		{
			name: "a method call on a continuation line",
			code: "<?php\nfunction f(Svc $svc) {\n    $p = $svc\n        ->point();\n    $p['‸\n}\n",
			want: []string{"x", "y"},
		},
		{
			name: "keys written one by one, split over lines",
			code: "<?php\nfunction f() {\n    $r = [];\n    $r[\n        'name'\n    ] = 1;\n    $r['count'] = 2;\n    $other['not'] = 3;\n    $r['‸\n}\n",
			want: []string{"name", "count"},
			not:  []string{"not"},
		},
		{
			name: "keys written in a heredoc or another function",
			code: "<?php\nfunction g() {\n    $r = [];\n    $r['other'] = 1;\n}\nfunction f() {\n    $r = [];\n    $s = <<<TXT\n$r['fake'] = 1;\nTXT;\n    $r['real'] = 2;\n    $r['‸\n}\n",
			want: []string{"real"},
			not:  []string{"fake", "other"},
		},
		{
			name: "a key written after the cursor",
			code: "<?php\nfunction f() {\n    $r = [];\n    echo $r['‸'];\n    $r['later'] = 1;\n}\n",
			want: []string{"later"},
		},
		{
			name: "a closure's own keys",
			code: "<?php\nfunction f() {\n    $r = [];\n    $r['outer'] = 1;\n    $g = function () { $r = []; $r['inner'] = 1; };\n    $r['‸\n}\n",
			want: []string{"outer"},
			not:  []string{"inner"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := completeMarked(p, tt.code)
			for _, w := range tt.want {
				if !labels[w] {
					t.Errorf("missing %q in %v", w, mapKeys(labels, 10))
				}
			}
			for _, n := range tt.not {
				if labels[n] {
					t.Errorf("unexpected %q in %v", n, mapKeys(labels, 10))
				}
			}
		})
	}
}

func TestArrayKeysOfFrameworkValuesAreTypedFromTheTree(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "app.php"), []byte("<?php\nreturn ['name' => 'x', 'db' => ['host' => 'h']];\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(dir, "StoreRequest.php")
	request := "<?php\nnamespace App;\nclass StoreRequest {\n    public function rules(): array {\n        return ['title' => 'required', 'body' => 'string'];\n    }\n    public function validated(): array { return []; }\n}\n"
	if err := os.WriteFile(requestPath, []byte(request), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFileWithSource("file://"+requestPath, request, symbols.SourceProject)

	laravel := NewProvider(idx, nil, "laravel")
	laravel.SetArrayResolver(models.NewFrameworkArrayResolver(idx, dir, "laravel"))
	tests := []struct {
		name string
		code string
		want []string
	}{
		{name: "config split over lines", code: "<?php\nfunction f() {\n    $c = config(\n        'app'\n    );\n    $c['‸\n}\n", want: []string{"name", "db"}},
		{name: "a config path", code: "<?php\nfunction f() {\n    $c = config('app.db');\n    $c['‸\n}\n", want: []string{"host"}},
		{name: "all config files", code: "<?php\nfunction f() {\n    $c = config();\n    $c['‸\n}\n", want: []string{"app"}},
		{name: "validated() on a typed request", code: "<?php\nfunction f(\\App\\StoreRequest $request) {\n    $data = $request\n        ->validated();\n    $data['‸\n}\n", want: []string{"title", "body"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := completeMarked(laravel, tt.code)
			for _, w := range tt.want {
				if !labels[w] {
					t.Errorf("missing %q in %v", w, mapKeys(labels, 10))
				}
			}
		})
	}

	for _, code := range []string{
		"<?php\nfunction f() {\n    $c = config($name);\n    $c['‸\n}\n",
		"<?php\nfunction f() {\n    $c = strtolower('app');\n    $c['‸\n}\n",
		"<?php\nfunction f($x) {\n    $c = config('app');\n    if ($x) {\n        $c = config('app.db');\n    }\n    $c['‸\n}\n",
	} {
		if labels := completeMarked(laravel, code); len(labels) != 0 {
			t.Errorf("%q: got %v", code, mapKeys(labels, 10))
		}
	}

	other := NewProvider(idx, nil, "none")
	other.SetArrayResolver(models.NewFrameworkArrayResolver(idx, dir, "none"))
	if labels := completeMarked(other, "<?php\nfunction f() {\n    $c = config('app');\n    $c['‸\n}\n"); labels["name"] {
		t.Errorf("config() outside Laravel: got %v", mapKeys(labels, 10))
	}
}
