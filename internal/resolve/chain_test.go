package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const chainSource = `<?php
namespace App;

use App\Models\Post as Article;

class Logger { public function info(): void {} }
class Person { public string $name = ''; public function logger(): Logger { return new Logger(); } }
/** @template T */
class Box {
    /** @param T $v */
    public function __construct($v) {}
    /** @return T */
    public function get() { return null; }
    /** @return static */
    public function copy() { return $this; }
}
class Base {
    /** @return static */
    public static function make() { return new static(); }
}
class Child extends Base {
    public function run(Person $p, Box $b) {
        $fn = fn (int $i): Person => new Person();
        /** @var array<string, Person> $people */
        $people = [];
        /** @var object{user: Person} $shape */
        $shape = (object) [];
        return 1;
    }
}
/**
 * @template T
 * @param T $a
 * @return T
 */
function identity($a) { return $a; }
`

func chainFixture(t *testing.T) (*Resolver, *parser.FileNode, protocol.Position) {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///chain.php", chainSource)
	file := parser.ParseFile(chainSource, token.Latest)
	line := strings.Count(chainSource[:strings.Index(chainSource, "return 1;")], "\n")
	return NewResolver(idx), file, protocol.Position{Line: line, Character: 8}
}

// The shared walker types each kind of chain the features ask about.
func TestChainType(t *testing.T) {
	r, file, pos := chainFixture(t)
	for expr, want := range map[string]string{
		"$p":                              `App\Person`,
		"$p->logger()":                    `App\Logger`,
		"$p?->logger()":                   `App\Logger`,
		"$p -> logger ()":                 `App\Logger`,
		"$this":                           `App\Child`,
		"self::make()":                    `App\Child`,
		"static::make()":                  `App\Child`,
		"PARENT::make()":                  `App\Child`,
		"Child::make()":                   `App\Child`,
		"Base::make()":                    `App\Base`,
		`\App\Person`:                     `App\Person`,
		"new Person()":                    `App\Person`,
		"(new Person)":                    `App\Person`,
		"(new Box(new Logger()))":         `App\Box<App\Logger>`,
		"(new Box(new Logger()))->get()":  `App\Logger`,
		"(new Box(new Logger()))->copy()": `App\Box<App\Logger>`,
		"identity(new Logger())":          `App\Logger`,
		"$fn":                             `Closure(int): App\Person`,
		"$fn(1)":                          `App\Person`,
		"$fn(1)->logger()":                `App\Logger`,
		"$fn(...)":                        `Closure(int): App\Person`,
		"$p->logger(...)":                 `Closure(): App\Logger`,
		"$people['a']":                    `App\Person`,
		"$people['a']->logger()":          `App\Logger`,
		"$shape->user":                    `App\Person`,
		"$shape->nope":                    ``,
		"$p->nope()":                      ``,
		"$missing->logger()":              ``,
		"Article":                         `App\Models\Post`,
		"":                                ``,
	} {
		if got := r.chainTypeOf(expr, chainSource, pos, file).FullString(); got != want {
			t.Errorf("ChainType(%q) = %q, want %q", expr, got, want)
		}
	}
}

// Without a file only class names can be read.
func TestChainTypeWithoutFile(t *testing.T) {
	r, _, pos := chainFixture(t)
	for expr, want := range map[string]string{
		"$this": ``, "self::make()": ``, "parent::make()": ``, "Person": `App\Person`, "$p->logger()": ``,
	} {
		if got := r.chainTypeOf(expr, chainSource, pos, nil).FullString(); got != want {
			t.Errorf("ChainType(%q) without a file = %q, want %q", expr, got, want)
		}
	}
}

// A container call answers through the ContainerCall hook.
func TestChainTypeContainerCall(t *testing.T) {
	r, file, pos := chainFixture(t)
	r.ContainerCall = func(_ *Resolver, src string, _ *parser.FileNode, call ast.Expr) (ResolvedType, bool) {
		switch call.Bounds().Text(src) {
		case "app('log')":
			return ResolvedType{FQN: `App\Logger`}, true
		case "config('a')":
			return ResolvedType{}, true
		}
		return ResolvedType{}, false
	}
	for expr, want := range map[string]string{
		"app('log')":       `App\Logger`,
		"app('log')->info": ``,
		"config('a')->x":   ``,
		"app('other')->x":  ``,
	} {
		if got := r.chainTypeOf(expr, chainSource, pos, file).FullString(); got != want {
			t.Errorf("ChainType(%q) = %q, want %q", expr, got, want)
		}
	}
}

func TestResolveClassNameLoose(t *testing.T) {
	r, file, _ := chainFixture(t)
	for name, want := range map[string]string{
		"":            "",
		`\Foo\Bar`:    `Foo\Bar`,
		"Article":     `App\Models\Post`,
		`Article\Tag`: `App\Models\Post\Tag`,
		"Person":      `App\Person`,
		"Logger":      `App\Logger`,
		"Unknown":     "Unknown",
	} {
		if got := r.ResolveClassNameLoose(name, file); got != want {
			t.Errorf("ResolveClassNameLoose(%q) = %q, want %q", name, got, want)
		}
	}
	// Without a file a short name is looked up in the index.
	if got := r.ResolveClassNameLoose("Person", nil); got != `App\Person` {
		t.Errorf("without a file = %q", got)
	}
	if got := r.ResolveClassNameLoose("Nowhere", nil); got != "Nowhere" {
		t.Errorf("unknown without a file = %q", got)
	}
	// A global class is found as written.
	global := parser.ParseFile("<?php\nnamespace Other;\n", token.Latest)
	if got := r.ResolveClassNameLoose(`App\Person`, global); got != `App\Person` {
		t.Errorf("a full name as written = %q", got)
	}
}

func TestResolveClassNameLooseFindsOnlyClasses(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///h.php", "<?php\nnamespace Helper;\nfunction person() {}\nclass Box { public function person() {} }\n")
	r := NewResolver(idx)
	file := parser.ParseFile("<?php\nnamespace App;\n", token.Latest)
	for _, f := range []*parser.FileNode{nil, file} {
		if got := r.ResolveClassNameLoose("person", f); got != "person" {
			t.Errorf("a function or method named like the class is not a class, got %q", got)
		}
		if got := r.ResolveClassNameLoose("Box", f); got != `Helper\Box` {
			t.Errorf("a class by its short name, got %q", got)
		}
	}
}

func TestClassKeyword(t *testing.T) {
	if got := ClassKeyword("Self"); got != "self" {
		t.Errorf("ClassKeyword = %q", got)
	}
}
