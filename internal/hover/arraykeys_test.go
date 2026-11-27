package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func hoverMarked(p *Provider, marked string) string {
	before, after, _ := strings.Cut(marked, "‸")
	line := strings.Count(before, "\n")
	col := len(before) - strings.LastIndexByte(before, '\n') - 1
	h := p.GetHover("file:///test.php", before+after, protocol.Position{Line: line, Character: col})
	if h == nil {
		return ""
	}
	return h.Contents.Value
}

func TestHoverArrayKeyCursorPositions(t *testing.T) {
	p := NewProvider(symbols.NewIndex(), nil, "")
	head := "<?php\nfunction f() {\n    $config = ['host' => 'h'];\n    "
	for name, code := range map[string]string{
		"single quote":     "$config['h‸ost'];",
		"double quote":     `$config["h‸ost"];`,
		"on the quote":     "$config[‸'host'];",
		"on the last char": "$config['hos‸t'];",
		"closing quote":    "$config['host‸'];",
	} {
		t.Run(name, func(t *testing.T) {
			if got := hoverMarked(p, head+code+"\n}\n"); !strings.Contains(got, "(array key) string $host") {
				t.Errorf("hover = %q, want the host key", got)
			}
		})
	}
	for name, code := range map[string]string{
		"integer key":       "$config[‸0];",
		"not an access":     "echo 'ho‸st';",
		"on the variable":   "$con‸fig['host'];",
		"after the bracket": "$config['host']‸;",
		"before the key":    "$config[‸ 'host'];",
		"an empty offset":   "$config[‸] = 1;",
		"a call's result":   "load()['h‸ost'];",
	} {
		t.Run(name, func(t *testing.T) {
			if got := hoverMarked(p, head+code+"\n}\n"); strings.Contains(got, "array key") {
				t.Errorf("hover = %q, want no array key", got)
			}
		})
	}
}

func TestHoverArrayKeysComeFromTheResolver(t *testing.T) {
	p := NewProvider(symbols.NewIndex(), nil, "")
	tests := []struct {
		name, code, want string
	}{
		{"an array split over lines", "<?php\nfunction f() {\n    $data = [\n        'name'\n            => 'n',\n        'age' => 3,\n    ];\n    $data['a‸ge'];\n}\n", "(array key) int $age"},
		{"an array() literal", "<?php\nfunction f() {\n    $data = array('name' => 'n');\n    $data['na‸me'];\n}\n", "(array key) string $name"},
		{"keys written one by one", "<?php\nfunction f() {\n    $data = [];\n    $data['name'] = 'n';\n    $data['na‸me'];\n}\n", "(array key) $name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hoverMarked(p, tt.code); !strings.Contains(got, tt.want) {
				t.Errorf("hover = %q, want %q", got, tt.want)
			}
		})
	}

	for name, code := range map[string]string{
		"a key in a comment":          "<?php\nfunction f() {\n    /*\n    $data = ['name' => 'n'];\n    */\n    $data = load();\n    $data['na‸me'];\n}\n",
		"a key in a string":           "<?php\nfunction f() {\n    $s = '\n    $data = [\"name\" => 1];\n    ';\n    $data = load();\n    $data['na‸me'];\n}\n",
		"an assignment in a function": "<?php\nfunction other() {\n    $data = ['name' => 'n'];\n}\nfunction f($data) {\n    $data['na‸me'];\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := hoverMarked(p, code); strings.Contains(got, "array key") {
				t.Errorf("hover = %q, want no array key", got)
			}
		})
	}
}

func TestHoverArrayKeyOfAConfigResult(t *testing.T) {
	p := setupConfigHoverProvider(t)
	got := hoverMarked(p, "<?php\nfunction f() {\n    $db = config('database.connections');\n    $db['mysql']['p‸ort'];\n}\n")
	if !strings.Contains(got, "(array key) int $port") {
		t.Errorf("hover = %q, want the port key of the config file", got)
	}
}

func TestHoverArrayKeyFromDocblock(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, nil, "")

	source := `<?php
/**
 * @param array{host: string, port: int, ssl?: bool} $config
 */
function connect(array $config): void {
    $config['host'];
}
`
	// Hover on 'host' at line 5
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 5, Character: 14})
	if hover == nil {
		t.Fatal("expected hover for array key 'host'")
	}
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Errorf("expected hover to contain type 'string', got %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "host") {
		t.Errorf("expected hover to contain key name 'host', got %q", hover.Contents.Value)
	}
}

func TestHoverArrayKeyOptional(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, nil, "")

	source := `<?php
/**
 * @param array{name: string, nickname?: string} $data
 */
function test(array $data): void {
    $data['nickname'];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 5, Character: 12})
	if hover == nil {
		t.Fatal("expected hover for optional array key")
	}
	if !strings.Contains(hover.Contents.Value, "optional") {
		t.Errorf("expected hover to indicate optional, got %q", hover.Contents.Value)
	}
}

func TestHoverArrayKeyFromLiteral(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $data = ['name' => 'John', 'age' => 30];
    $data['name'];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 3, Character: 12})
	if hover == nil {
		t.Fatal("expected hover for literal array key")
	}
	if !strings.Contains(hover.Contents.Value, "name") {
		t.Errorf("expected hover to contain 'name', got %q", hover.Contents.Value)
	}
}

func TestHoverArrayDeclarationKeyDoesNotResolveStandaloneSymbol(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Application.php", `<?php
namespace Illuminate\Contracts\Foundation;

class Application {
    public function environment(...$environments): string|bool {}
}
`)
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $services = [
        'environment' => [
            'DOCKER_TLS_CERTDIR=',
        ],
    ];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 3, Character: 11})
	if hover != nil {
		t.Fatalf("expected no hover for array declaration key, got:\n%s", hover.Contents.Value)
	}
}

func TestHoverArrayKeyFromVarAnnotation(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    /** @var array{id: int, title: string} $post */
    $post = getPost();
    $post['title'];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 4, Character: 12})
	if hover == nil {
		t.Fatal("expected hover for @var array key")
	}
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Errorf("expected type 'string' in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverNestedArrayKey(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, nil, "")

	source := `<?php
/**
 * @param array{database: array{host: string, port: int}, cache: array{driver: string}} $config
 */
function connect(array $config): void {
    $config['database']['host'];
}
`
	// Hover on 'host' inside $config['database']['host']
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 5, Character: 27})
	if hover == nil {
		t.Fatal("expected hover for nested array key 'host'")
	}
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Errorf("expected type 'string' in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverNestedArrayKeyWrongParent(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, nil, "")

	source := `<?php
/**
 * @param array{database: array{host: string}, cache: array{driver: string}} $config
 */
function connect(array $config): void {
    $config['cache']['host'];
}
`
	// 'host' doesn't exist in cache shape, only in database
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 5, Character: 24})
	if hover != nil && strings.Contains(hover.Contents.Value, "array key") {
		t.Error("should not show array key hover for 'host' in cache (wrong parent)")
	}
}

func TestHoverNestedLiteralArrayKey(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $config = [
        'database' => [
            'host' => 'localhost',
            'port' => 3306,
        ],
    ];
    $config['database']['host'];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 8, Character: 27})
	if hover == nil {
		t.Fatal("expected hover for nested literal array key 'host'")
	}
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Errorf("expected type 'string' in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverArrayKeyNonExistent(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $data = ['name' => 'John'];
    $data['unknown'];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 3, Character: 12})
	if hover != nil {
		// Should not contain type info for non-existent key
		if strings.Contains(hover.Contents.Value, "array key") {
			t.Error("expected no array key hover for non-existent key")
		}
	}
}
