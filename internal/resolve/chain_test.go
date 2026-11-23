package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestForwardsCallingClass(t *testing.T) {
	for before, want := range map[string]bool{
		"parent::": true, "  self :: ": true, "return STATIC::": true,
		"Base::": false, "$this->": false, "$parent::": false, "": false,
	} {
		if got := ForwardsCallingClass(before); got != want {
			t.Errorf("ForwardsCallingClass(%q) = %v, want %v", before, got, want)
		}
	}
}

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
	file := parser.ParseFile(chainSource)
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
		if got := r.ChainType(expr, chainSource, pos, file).FullString(); got != want {
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
		if got := r.ChainType(expr, chainSource, pos, nil).FullString(); got != want {
			t.Errorf("ChainType(%q) without a file = %q, want %q", expr, got, want)
		}
	}
}

// A container call answers through the ContainerCall hook.
func TestChainTypeContainerCall(t *testing.T) {
	r, file, pos := chainFixture(t)
	r.ContainerCall = func(callExpr, source string, file *parser.FileNode) string {
		switch callExpr {
		case "app('log')":
			return `App\Logger`
		case "config('a')":
			return "-"
		}
		return ""
	}
	for expr, want := range map[string]string{
		"app('log')":       `App\Logger`,
		"app('log')->info": ``,
		"config('a')->x":   ``,
		"app('other')->x":  ``,
	} {
		if got := r.ChainType(expr, chainSource, pos, file).FullString(); got != want {
			t.Errorf("ChainType(%q) = %q, want %q", expr, got, want)
		}
	}
}

// A resolver wired to another one asks it, for whole chains and receivers.
func TestChainTypeWired(t *testing.T) {
	r, file, pos := chainFixture(t)
	var asked []string
	r.TypedChainResolver = func(expr, source string, pos protocol.Position, file *parser.FileNode) ResolvedType {
		asked = append(asked, expr)
		return ResolvedType{FQN: `App\Logger`}
	}
	if got := r.ChainType("$p->logger()", chainSource, pos, file).FQN; got != `App\Logger` {
		t.Errorf("ChainType = %q", got)
	}
	// A slice, not a map: the order of the calls is checked below.
	for _, tc := range []struct {
		line string
		ws   int
	}{{"$p->info", 4}, {"$p?->info", 5}, {"Person :: make", 10}} {
		if got := r.AccessOwnerType(tc.line, tc.ws, chainSource, pos, file).FQN; got != `App\Logger` {
			t.Errorf("AccessOwnerType(%q) = %q", tc.line, got)
		}
	}
	if got := r.AccessOwnerType("info", 0, chainSource, pos, file); !got.IsEmpty() {
		t.Errorf("AccessOwnerType without an operator = %v", got)
	}
	want := []string{"$p->logger()", "$p", "$p", "Person "}
	if strings.Join(asked, ",") != strings.Join(want, ",") {
		t.Errorf("asked %q, want %q", asked, want)
	}
	r.TypedChainResolver = nil
	if got := r.AccessOwnerType("$p->logger", 4, chainSource, pos, file).FQN; got != `App\Person` {
		t.Errorf("own AccessOwnerType = %q", got)
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
	global := parser.ParseFile("<?php\nnamespace Other;\n")
	if got := r.ResolveClassNameLoose(`App\Person`, global); got != `App\Person` {
		t.Errorf("a full name as written = %q", got)
	}
}

func TestExtractNewClassAndArgs(t *testing.T) {
	for in, want := range map[string][2]string{
		"new Foo()":       {"Foo", ""},
		"(new Foo(1, 2))": {"Foo", "1, 2"},
		"new Foo":         {"Foo", ""},
		"(new Foo)":       {"Foo", ""},
		"(new Foo((1)))":  {"Foo", "(1)"},
		"foo()":           {"", ""},
	} {
		if got := ExtractNewClass(in); got != want[0] {
			t.Errorf("ExtractNewClass(%q) = %q, want %q", in, got, want[0])
		}
		if got := extractNewArgs(in); got != want[1] {
			t.Errorf("extractNewArgs(%q) = %q, want %q", in, got, want[1])
		}
	}
}

func TestChainOriginClassName(t *testing.T) {
	r, file, _ := chainFixture(t)
	if got := r.ChainOriginClassName("Person::make", 0, "Person", file); got != `App\Person` {
		t.Errorf("origin = %q", got)
	}
	// A word after -> or :: is a member, never a class.
	if got := r.ChainOriginClassName("$x->Person", 4, "Person", file); got != "" {
		t.Errorf("member = %q", got)
	}
	if got := ClassKeyword("Self"); got != "self" {
		t.Errorf("ClassKeyword = %q", got)
	}
}
