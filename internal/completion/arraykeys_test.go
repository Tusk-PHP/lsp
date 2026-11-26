package completion

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func arrayKeyContextAt(t *testing.T, marked string) (keys *arrayKeyContext, varName string, ok bool) {
	t.Helper()
	ctx := cursorContextAt(t, marked)
	dim, ok := ctx.StringDim()
	if !ok {
		return nil, "", false
	}
	keys, ok = arrayKeyContextOf(dim)
	if ok {
		varName = keys.Var.Bounds().Text(ctx.File.Source)
	}
	return keys, varName, ok
}

func TestArrayKeyContext(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		wantVar    string
		wantAccess []string
		wantPart   string
		wantQuote  string
		wantOk     bool
	}{
		{name: "single quote start", code: "$config['", wantVar: "$config", wantQuote: "'", wantOk: true},
		{name: "double quote start", code: `$config["`, wantVar: "$config", wantQuote: "\"", wantOk: true},
		{name: "partial key", code: "$config['da", wantVar: "$config", wantPart: "da", wantQuote: "'", wantOk: true},
		{name: "bracket only", code: "$config[", wantVar: "$config", wantOk: true},
		{name: "nested in call", code: "foo($config['k", wantVar: "$config", wantPart: "k", wantQuote: "'", wantOk: true},
		{name: "two levels", code: "$config['database']['", wantVar: "$config", wantAccess: []string{"database"}, wantQuote: "'", wantOk: true},
		{name: "three levels", code: "$config['database']['replicas']['", wantVar: "$config", wantAccess: []string{"database", "replicas"}, wantQuote: "'", wantOk: true},
		{name: "a level on the next line", code: "$config['database']\n    ['repl", wantVar: "$config", wantAccess: []string{"database"}, wantPart: "repl", wantQuote: "'", wantOk: true},
		{name: "inside a closed key", code: "$config['da|ta'];", wantVar: "$config", wantPart: "da", wantQuote: "'", wantOk: true},
		{name: "not array access", code: "foo('bar"},
		{name: "no variable", code: "['key"},
		{name: "method call not var", code: "$this->method('"},
		{name: "a property", code: "$this->data['"},
		{name: "a level that is not a string", code: "$config[$k]['"},
		{name: "in a comment", code: "// $config['"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, varName, ok := arrayKeyContextAt(t, "<?php\n"+tt.code)
			if ok != tt.wantOk {
				t.Fatalf("ok = %v, want %v (%#v)", ok, tt.wantOk, ctx)
			}
			if !ok {
				return
			}
			if varName != tt.wantVar || ctx.Partial != tt.wantPart || ctx.Quote != tt.wantQuote || !slices.Equal(ctx.AccessKeys, tt.wantAccess) {
				t.Fatalf("got %#v", ctx)
			}
		})
	}
}

func TestCompleteArrayKeysFromDocblock(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///test.php", `<?php
namespace App;
class Service {
    /**
     * @param array{host: string, port: int, ssl?: bool} $config
     */
    public function connect(array $config): void {}
}
`)
	p := NewProvider(idx, nil, "")

	source := `<?php
namespace App;
class Service {
    /**
     * @param array{host: string, port: int, ssl?: bool} $config
     */
    public function connect(array $config): void {
        $config['
    }
}
`
	// Cursor at $config[' — line 7, after the quote
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 7, Character: 17})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["host"] {
		t.Error("expected 'host' key completion")
	}
	if !labels["port"] {
		t.Error("expected 'port' key completion")
	}
	if !labels["ssl"] {
		t.Error("expected 'ssl' key completion")
	}

	// Check ssl is marked optional
	for _, item := range items {
		if item.Label == "ssl" {
			if item.SortText[0:1] != "1" {
				t.Errorf("optional key 'ssl' should sort after required keys, got SortText %q", item.SortText)
			}
		}
	}
}

func TestCompleteArrayKeysFromLiteralAssignment(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $data = [
        'name' => 'John',
        'email' => 'john@example.com',
        'age' => 30,
    ];
    $data['
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 7, Character: 11})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["name"] {
		t.Error("expected 'name' from literal assignment")
	}
	if !labels["email"] {
		t.Error("expected 'email' from literal assignment")
	}
	if !labels["age"] {
		t.Error("expected 'age' from literal assignment")
	}
}

func TestCompleteArrayKeysFromIncrementalAssignment(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $result = [];
    $result['name'] = 'test';
    $result['count'] = 42;
    $result['
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 5, Character: 13})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["name"] {
		t.Error("expected 'name' from incremental assignment")
	}
	if !labels["count"] {
		t.Error("expected 'count' from incremental assignment")
	}
}

func TestCompleteArrayKeysFiltersOnPartial(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $config = ['host' => 'x', 'port' => 3306, 'hostname' => 'y'];
    $config['ho
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 15})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["host"] {
		t.Error("expected 'host' matching 'ho' prefix")
	}
	if !labels["hostname"] {
		t.Error("expected 'hostname' matching 'ho' prefix")
	}
	if labels["port"] {
		t.Error("should NOT show 'port' (doesn't match 'ho')")
	}
}

func TestCompleteArrayKeysFromVarAnnotation(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    /** @var array{id: int, title: string, published?: bool} $post */
    $post = getPost();
    $post['
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 4, Character: 11})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["id"] {
		t.Error("expected 'id' from @var annotation")
	}
	if !labels["title"] {
		t.Error("expected 'title' from @var annotation")
	}
	if !labels["published"] {
		t.Error("expected 'published' from @var annotation")
	}
}

func TestCompleteNestedArrayKeys(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///test.php", `<?php
namespace App;
class Service {
    /**
     * @param array{database: array{host: string, port: int}, cache: array{driver: string, ttl: int}} $config
     */
    public function connect(array $config): void {}
}
`)
	p := NewProvider(idx, nil, "")

	source := `<?php
namespace App;
class Service {
    /**
     * @param array{database: array{host: string, port: int}, cache: array{driver: string, ttl: int}} $config
     */
    public function connect(array $config): void {
        $config['database']['
    }
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 7, Character: 29})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["host"] {
		t.Error("expected 'host' from nested array{database: array{host: string}}")
	}
	if !labels["port"] {
		t.Error("expected 'port' from nested shape")
	}
	if labels["driver"] {
		t.Error("should NOT show 'driver' (that's in cache, not database)")
	}
	if labels["database"] {
		t.Error("should NOT show 'database' (that's the top level)")
	}
}

func TestCompleteNestedArrayKeysTopLevel(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///test.php", `<?php
namespace App;
class Service {
    /**
     * @param array{database: array{host: string}, cache: array{driver: string}} $config
     */
    public function connect(array $config): void {}
}
`)
	p := NewProvider(idx, nil, "")

	source := `<?php
namespace App;
class Service {
    /**
     * @param array{database: array{host: string}, cache: array{driver: string}} $config
     */
    public function connect(array $config): void {
        $config['
    }
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 7, Character: 17})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["database"] {
		t.Error("expected 'database' at top level")
	}
	if !labels["cache"] {
		t.Error("expected 'cache' at top level")
	}
}

func TestCompleteNestedLiteralArrayKeys(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $config = [
        'database' => [
            'host' => 'localhost',
            'port' => 3306,
        ],
        'cache' => [
            'driver' => 'redis',
        ],
    ];
    $config['database']['
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 11, Character: 25})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["host"] {
		t.Error("expected 'host' from nested literal array")
	}
	if !labels["port"] {
		t.Error("expected 'port' from nested literal array")
	}
	if labels["driver"] {
		t.Error("should NOT show 'driver' (that's in cache, not database)")
	}
	if labels["database"] {
		t.Error("should NOT show 'database' (that's the top level)")
	}
}

func TestCompleteArrayKeysQuotingBehavior(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $arr = ['key1' => 1, 'key2' => 2];
    $arr[
}
`
	// No quote typed yet — bracket only
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 9})

	if len(items) > 0 {
		// Should wrap keys in quotes
		for _, item := range items {
			if item.InsertText != "" && item.InsertText[0] != '\'' {
				t.Errorf("expected quote-wrapped key, got InsertText %q", item.InsertText)
			}
		}
	}
}
