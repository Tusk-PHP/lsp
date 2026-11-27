package completion

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func setupConfigProvider(t *testing.T) (*Provider, string) {
	t.Helper()
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "config"), 0755)
	os.WriteFile(filepath.Join(dir, "config", "app.php"), []byte(`<?php
return [
    'name' => 'My App',
    'debug' => true,
    'url' => 'http://localhost',
];
`), 0644)
	os.WriteFile(filepath.Join(dir, "config", "database.php"), []byte(`<?php
return [
    'default' => 'mysql',
    'connections' => [
        'mysql' => [
            'host' => 'localhost',
            'port' => 3306,
        ],
        'sqlite' => [
            'driver' => 'sqlite',
            'database' => ':memory:',
        ],
    ],
];
`), 0644)
	os.WriteFile(filepath.Join(dir, "config", "cache.php"), []byte(`<?php
return [
    'default' => 'file',
    'stores' => [
        'file' => ['driver' => 'file', 'path' => '/tmp'],
    ],
];
`), 0644)

	idx := symbols.NewIndex()
	resolver := models.NewFrameworkArrayResolver(idx, dir, "laravel")
	p := NewProvider(idx, nil, "laravel")
	p.SetArrayResolver(resolver)
	return p, dir
}

func TestConfigArgContext(t *testing.T) {
	tests := []struct {
		name        string
		trimmed     string
		wantPath    string
		wantPartial string
		wantQuote   string
		wantOk      bool
	}{
		{"empty single quote", "config('", "", "", "'", true},
		{"empty double quote", `config("`, "", "", "\"", true},
		{"partial top level", "config('da", "", "da", "'", true},
		{"top level complete dot", "config('database.", "database", "", "'", true},
		{"nested partial", "config('database.co", "database", "co", "'", true},
		{"deep nested", "config('database.connections.", "database.connections", "", "'", true},
		{"deep nested partial", "config('database.connections.my", "database.connections", "my", "'", true},
		{"closed string", "config('database.co|')", "database", "co", "'", true},
		{"no quote", "config(", "", "", "", true},
		{"argument on the next line", "config(\n    'database.|'\n);", "database", "", "'", true},
		{"closed paren", "config('database')", "", "", "", false},
		{"second argument", "config('database', '", "", "", "", false},
		{"not config", "app('something", "", "", "", false},
		{"a method named config", "$this->config('da", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, ok := callArgAt(t, "<?php\n"+tt.trimmed)
			var path, partial, quote string
			if ok {
				path, partial, quote, ok = configArgContext(arg)
			}
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !ok {
				return
			}
			if path != tt.wantPath {
				t.Errorf("configPath = %q, want %q", path, tt.wantPath)
			}
			if partial != tt.wantPartial {
				t.Errorf("partial = %q, want %q", partial, tt.wantPartial)
			}
			if quote != tt.wantQuote {
				t.Errorf("quote = %q, want %q", quote, tt.wantQuote)
			}
		})
	}
}

func TestCompleteConfigTopLevel(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 8})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["app"] {
		t.Error("expected 'app' config file")
	}
	if !labels["database"] {
		t.Error("expected 'database' config file")
	}
	if !labels["cache"] {
		t.Error("expected 'cache' config file")
	}
}

func TestCompleteConfigTopLevelFilter(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('da
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 10})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["database"] {
		t.Error("expected 'database' matching 'da'")
	}
	if labels["app"] {
		t.Error("should NOT show 'app' (doesn't match 'da')")
	}
}

func TestCompleteConfigNestedKeys(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 17})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["default"] {
		t.Error("expected 'default' from database config")
	}
	if !labels["connections"] {
		t.Error("expected 'connections' from database config")
	}
}

func TestCompleteConfigDeepNested(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.connections.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 29})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["mysql"] {
		t.Error("expected 'mysql' connection")
	}
	if !labels["sqlite"] {
		t.Error("expected 'sqlite' connection")
	}
}

func TestCompleteConfigDeepNestedPartial(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.connections.my
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 31})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["mysql"] {
		t.Error("expected 'mysql' matching 'my'")
	}
	if labels["sqlite"] {
		t.Error("should NOT show 'sqlite' (doesn't match 'my')")
	}
}

func TestCompleteConfigNestedAppendsTrailingDot(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 17})

	for _, item := range items {
		if item.Label == "connections" {
			if item.InsertText != "connections." {
				t.Errorf("expected InsertText 'connections.' for nested key, got %q", item.InsertText)
			}
			if item.Kind != protocol.CompletionItemKindModule {
				t.Errorf("expected Module kind for nested config, got %d", item.Kind)
			}
			return
		}
	}
	t.Error("'connections' not found in completions")
}

func TestCompleteConfigLeafNoTrailingDot(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 17})

	for _, item := range items {
		if item.Label == "default" {
			if item.InsertText != "default" {
				t.Errorf("expected InsertText 'default' for leaf key, got %q", item.InsertText)
			}
			if item.Kind != protocol.CompletionItemKindProperty {
				t.Errorf("expected Property kind for leaf config, got %d", item.Kind)
			}
			return
		}
	}
	t.Error("'default' not found in completions")
}

func TestCompleteConfigNestedShowsSummaryDetail(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 17})

	for _, item := range items {
		if item.Label == "connections" {
			// Should show a summary of nested keys, not the raw array{...} type
			if !strings.Contains(item.Detail, "mysql") {
				t.Errorf("expected 'mysql' in detail summary, got %q", item.Detail)
			}
			if !strings.Contains(item.Detail, "sqlite") {
				t.Errorf("expected 'sqlite' in detail summary, got %q", item.Detail)
			}
			return
		}
	}
	t.Error("'connections' not found")
}

func TestConfigResultContext(t *testing.T) {
	for code, want := range map[string]*configResultArrayContext{
		"config('database')['connections']['my": {ConfigArg: "database", AccessKeys: []string{"connections"}, Partial: "my", Quote: "'"},
		"config('database')[":                   {ConfigArg: "database"},
		"config('database')[$k]['":              nil,
		"config($key)['":                        nil,
		"config('')['":                          nil,
		"config()['":                            nil,
		"other('database')['":                   nil,
		"$config['":                             nil,
	} {
		source := "<?php\n" + code
		ctx := sourcectx.Analyze("file:///test.php", source, protocol.Position{Line: 1, Character: len(code)}, phpparser.OnlyGrammar(token.Latest))
		var got *configResultArrayContext
		if dim, ok := ctx.StringDim(); ok {
			got = configResultContext(ctx.File.Source, dim)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %#v, want %#v", code, got, want)
		}
	}
}

func TestConfigResultKeysNeedALiteralConfigKey(t *testing.T) {
	p, _ := setupConfigProvider(t)
	for _, source := range []string{"<?php\nconfig($key)['", "<?php\nconfig('')['", "<?php\nconfig('database')[$k]['", "<?php\nother('database')['"} {
		for _, item := range p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: len(source) - len("<?php\n")}) {
			if item.Label == "connections" || item.Label == "default" {
				t.Errorf("%q: unexpected config key %q", source, item.Label)
			}
		}
	}
}

func TestCompleteConfigDoesNotBreakContainerCompletion(t *testing.T) {
	// app() should still trigger container completion, not config
	arg, ok := callArgAt(t, "<?php\napp('re")
	var path, quote string
	if ok {
		_, path, quote, ok = configArgContext(arg)
	}
	if ok {
		t.Errorf("app() should NOT trigger config context, got path=%q quote=%q", path, quote)
	}
}

func TestCompleteConfigNoQuoteWrapsInQuotes(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config(
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 7})

	if len(items) == 0 {
		t.Fatal("expected config suggestions when no quote typed")
	}

	for _, item := range items {
		if item.Label == "database" {
			if item.InsertText == "" || item.InsertText[0] != '\'' {
				t.Errorf("expected quote-wrapped InsertText, got %q", item.InsertText)
			}
			return
		}
	}
	t.Error("'database' not found in config( completions")
}

func TestCompleteConfigWithArgReturnsNoMemberAccess(t *testing.T) {
	// config('database')-> should NOT offer Repository methods
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	// Index a fake Repository class
	idx.IndexFileWithSource("file:///repo.php", `<?php
namespace Illuminate\Config;
class Repository {
    public function get(string $key): mixed {}
    public function set(string $key, mixed $value): void {}
    public function all(): array {}
    public function has(string $key): bool {}
}
`, symbols.SourceVendor)

	p := NewProvider(idx, nil, "laravel")

	source := `<?php
config('database')->
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 20})

	// Should return nil/empty — config('key') returns mixed, not Repository
	for _, item := range items {
		if item.Label == "get" || item.Label == "set" || item.Label == "all" || item.Label == "has" {
			t.Errorf("should NOT show Repository method %q for config('database')->", item.Label)
		}
	}
}

func TestCompleteConfigNoArgReturnsMethods(t *testing.T) {
	// config()-> SHOULD offer Repository methods
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFileWithSource("file:///repo.php", `<?php
namespace Illuminate\Config;
class Repository {
    public function get(string $key): mixed {}
    public function all(): array {}
}
`, symbols.SourceVendor)

	ca := container.NewContainerAnalyzer(idx, "/tmp", "laravel")
	ca.Analyze()
	p := NewProvider(idx, ca, "laravel")

	source := `<?php
config()->
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 10})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["get"] {
		t.Error("expected 'get' method for config()->")
	}
	if !labels["all"] {
		t.Error("expected 'all' method for config()->")
	}
}

func TestCompleteConfigDoubleQuotes(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config("database.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 17})

	if len(items) == 0 {
		t.Fatal("expected completions with double quotes")
	}

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["connections"] {
		t.Error("expected 'connections' with double quotes")
	}
}

func assignedValue(t *testing.T, src string) (*parser.FileNode, ast.Expr) {
	t.Helper()
	file := parser.ParseFile(src, token.Latest)
	var value ast.Expr
	ast.Walk(file.Tree, func(n ast.Node) bool {
		if assign, ok := n.(*ast.Assign); ok && value == nil {
			value = assign.Value
		}
		return value == nil
	})
	if value == nil {
		t.Fatalf("no assignment in %q", src)
	}
	return file, value
}

func TestCompletionAndContainerAgreeOnConfigCalls(t *testing.T) {
	p, dir := setupConfigProvider(t)
	ca := container.NewContainerAnalyzer(p.index, dir, "laravel")
	ca.Analyze()
	r := container.NewEngine(p.index, ca).Resolver()
	tests := []struct {
		call     string
		isConfig bool
		key      string
	}{
		{call: "config('app')", isConfig: true, key: "name"},
		{call: `\config("app")`, isConfig: true, key: "name"},
		{call: "config()", isConfig: true, key: "app"},
		{call: "$this->config('app')"},
		{call: "Config::get('app')"},
		{call: "configure('app')"},
	}
	for _, tt := range tests {
		t.Run(tt.call, func(t *testing.T) {
			src := "<?php\nfunction f() {\n    $c = " + tt.call + ";\n    $c['‸\n}\n"
			file, value := assignedValue(t, strings.Replace(src, "‸", "", 1))
			rt, typed := ca.CallType(r, file.Source, file, value)
			if typed != tt.isConfig {
				t.Fatalf("container typed %q: %v (%q), want %v", tt.call, typed, rt.FQN, tt.isConfig)
			}
			if want := tt.call == "config()"; typed && (rt.FQN == `Illuminate\Config\Repository`) != want {
				t.Fatalf("container typed %q as %q", tt.call, rt.FQN)
			}
			labels := completeMarked(p, src)
			if tt.isConfig != labels[tt.key] && tt.key != "" {
				t.Fatalf("completion of %q offers %q: %v, want %v (%v)", tt.call, tt.key, labels[tt.key], tt.isConfig, mapKeys(labels, 10))
			}
			if !tt.isConfig && (labels["name"] || labels["app"]) {
				t.Fatalf("completion of %q offers config keys: %v", tt.call, mapKeys(labels, 10))
			}
		})
	}
}
