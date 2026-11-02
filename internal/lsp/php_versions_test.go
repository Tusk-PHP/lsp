package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// phpVersionCase checks one PHP language feature through the real server, so a
// README "Understood" claim is backed by what an editor would actually see.
// The cursor sits at the "<|>" marker in src.
type phpVersionCase struct {
	name string
	src  string
	// hover, completion and definition requests run only when their want is set.
	hover      []string
	completion []string
	definition bool
	// noDiag lists diagnostic codes that must not appear; diag those that must.
	noDiag []string
	diag   []string
	// clean asserts the file produces no diagnostics at all.
	clean bool
}

const phpVersionPrelude = `<?php
namespace App;

class Logger { public function info(string $m): void {} public int $level = 0; const LEVEL = 1; }
interface Named { public function name(): string; }
interface Aged { public function age(): int; }
class Person implements Named, Aged {
	public function name(): string { return ''; }
	public function age(): int { return 0; }
}
`

var phpVersionCases = []phpVersionCase{
	// PHP 7.4
	{name: "7.4/null coalescing assignment defines the variable", clean: true, src: `
function run(): Logger { $log ??= new Logger(); return $log; }`},
	{name: "7.4/null coalescing assignment infers the type", hover: []string{"info"}, src: `
function run(): void { $log ??= new Logger(); $log->in<|>fo('x'); }`},
	{name: "7.4/array spread counts as a read", clean: true, src: `
function run(array $a): array { $b = [1]; return [...$a, ...$b, 2]; }`},
	{name: "7.4/array spread keeps the element type", completion: []string{"info"}, src: `
/** @param Logger[] $a */
function run(array $a): void { $b = [...$a]; foreach ($b as $l) { $l-><|> } }`},

	// PHP 8.0
	{name: "8.0/throw expression after ?? is not unreachable", clean: true, src: `
function run(?Logger $a): Logger { $x = $a ?? throw new \RuntimeException('no'); return $x; }`},
	{name: "8.0/throw expression after ?: is not unreachable", clean: true, src: `
function run(?Logger $a): ?Logger { $a ?: throw new \RuntimeException('no'); return $a; }`},
	{name: "8.0/throw expression in an arrow function", clean: true, src: `
function run(): \Closure { $f = fn () => throw new \RuntimeException('no'); return $f; }`},
	{name: "8.0/throw expression in a match arm", clean: true, src: `
function run(int $a): int { $b = match ($a) { 1 => 2, default => throw new \RuntimeException('no') }; return $b; }`},
	{name: "8.0/throw expression class is checked", diag: []string{"unknown-class"}, src: `
function run(?Logger $a): Logger { return $a ?? throw new Missing('no'); }`},
	{name: "8.0/throw statement still ends the block", diag: []string{"unreachable-code"}, src: `
function run(): void { throw new \RuntimeException('no'); echo 1; }`},
	{name: "8.0/type survives a throw expression", hover: []string{"info"}, src: `
function run(?Logger $a): void { $x = $a ?? throw new \RuntimeException('no'); $x->in<|>fo('x'); }`},
	{name: "8.0/promoted property completes", completion: []string{"log"}, src: `
class Svc { public function __construct(private Logger $log) {} function go(): void { $this-><|> } }`},
	{name: "8.0/promoted property has a type", hover: []string{"info"}, src: `
class Svc { public function __construct(private Logger $log) {} function go(): void { $this->log->in<|>fo('x'); } }`},
	{name: "8.0/promoted property goes to its parameter", definition: true, src: `
class Svc { public function __construct(private Logger $log) {} function go(): void { $this->l<|>og->info('x'); } }`},
	{name: "8.0/promoted readonly shows in hover", hover: []string{"readonly"}, src: `
class Svc { public function __construct(private readonly Logger $log) {} function go(): void { $this->l<|>og; } }`},
	{name: "8.0/attribute class goes to its definition", definition: true, src: `
#[\Attribute] class Marker {}
#[Mar<|>ker] class Svc {}`},
	{name: "8.0/attribute class shows in hover", hover: []string{"Marker"}, src: `
#[\Attribute] class Marker {}
#[Mar<|>ker] class Svc {}`},
	{name: "8.0/attribute on a method shows in its hover", hover: []string{"#[Marker]"}, src: `
#[\Attribute] class Marker {}
class Svc { #[Marker] public function go(): void {} function run(): void { $this->g<|>o(); } }`},
	{name: "8.0/unknown attribute is reported", diag: []string{"unknown-class"}, src: `
#[Missing] class Svc {}`},

	// PHP 8.1
	{name: "8.1/enum method completes", completion: []string{"label"}, src: `
enum Suit: string { case Hearts = 'h'; public function label(): string { return ''; } }
function run(Suit $s): void { $s-><|> }`},
	{name: "8.1/enum method completes from a case", completion: []string{"label"}, src: `
enum Suit: string { case Hearts = 'h'; public function label(): string { return ''; } }
function run(): void { Suit::Hearts-><|> }`},
	{name: "8.1/enum method shows in hover", hover: []string{"string"}, src: `
enum Suit: string { case Hearts = 'h'; public function label(): string { return ''; } }
function run(Suit $s): void { $s->la<|>bel(); }`},
	{name: "8.1/enum method goes to its definition", definition: true, src: `
enum Suit: string { case Hearts = 'h'; public function label(): string { return ''; } }
function run(Suit $s): void { $s->la<|>bel(); }`},
	{name: "8.1/enum static method completes", completion: []string{"fromCode"}, src: `
enum Suit { case Hearts; public static function fromCode(): self { return self::Hearts; } }
function run(): void { Suit::<|> }`},
	{name: "8.1/enum method return type flows", hover: []string{"info"}, src: `
enum Suit { case Hearts; public function log(): Logger { return new Logger(); } }
function run(Suit $s): void { $s->log()->in<|>fo('x'); }`},
	{name: "8.1/enum constant completes", completion: []string{"Wild"}, src: `
enum Suit { case Hearts; const Wild = self::Hearts; }
function run(): void { Suit::<|> }`},
	{name: "8.1/enum trait method completes", completion: []string{"shared"}, src: `
trait Helps { public function shared(): void {} }
enum Suit { use Helps; case Hearts; }
function run(Suit $s): void { $s-><|> }`},
	{name: "8.1/backed enum builtins complete", completion: []string{"from", "tryFrom", "cases"}, src: `
enum Suit: string { case Hearts = 'h'; }
function run(): void { Suit::<|> }`},
	{name: "8.1/enum case properties complete", completion: []string{"name", "value"}, src: `
enum Suit: string { case Hearts = 'h'; }
function run(Suit $s): void { $s-><|> }`},
	{name: "8.1/enum implementing an interface is clean", clean: true, src: `
enum Suit implements Named { case Hearts; public function name(): string { return ''; } }`},
	{name: "8.1/enum method body is checked", diag: []string{"undefined-variable"}, src: `
enum Suit { case Hearts; public function label(): string { return $nope; } }`},
	{name: "8.1/case named like a keyword does not end the enum", completion: []string{"after"}, src: `
enum Mode { case Default; case Static; public function after(): void {} }
function run(Mode $m): void { $m-><|> }`},
	{name: "8.1/readonly property shows in hover", hover: []string{"readonly"}, src: `
class Svc { public readonly Logger $log; function go(): void { $this->l<|>og; } }`},
	{name: "8.1/new in initializer is clean", clean: true, src: `
class Svc { public function __construct(private Logger $log = new Logger()) {} function go(): Logger { return $this->log; } }`},
	{name: "8.1/new in initializer with arguments keeps later parameters", completion: []string{"second"}, src: `
class Pair { public function __construct(public int $a = 0, public int $b = 0) {} }
class Svc { public function __construct(private Pair $p = new Pair(1, 2), private int $second = 3) {} function go(): void { $this-><|> } }`},
	{name: "8.1/new in initializer class is checked", diag: []string{"unknown-class"}, src: `
class Svc { public function __construct(private Logger $log = new Missing()) {} }`},
	{name: "8.1/new in initializer goes to the class", definition: true, src: `
class Svc { public function __construct(private Logger $log = new Log<|>ger()) {} }`},
	{name: "8.1/new in initializer types an untyped parameter", hover: []string{"info"}, src: `
function run($log = new Logger()): void { $log->in<|>fo('x'); }`},
	{name: "8.1/new in a static variable", hover: []string{"info"}, src: `
function run(): void { static $log = new Logger(); $log->in<|>fo('x'); }`},

	// PHP 8.2
	{name: "8.2/DNF parameter keeps its name and type", completion: []string{"name", "age"}, src: `
function run((Named&Aged)|null $x): void { $x-><|> }`},
	{name: "8.2/DNF type keeps its shape in hover", hover: []string{"(Named \\u0026 Aged) | null"}, src: `
class Svc { private (Named&Aged)|null $x = null; function go(): void { $this-><|>x; } }`},
	{name: "8.1/intersection type has both sides' members", completion: []string{"name", "age"}, src: `
function run(Named&Aged $x): void { $x-><|> }`},
	{name: "8.2/DNF function is clean", clean: true, src: `
function run((Named&Aged)|null $x, int $y): (Named&Aged)|null { echo $y; return $x; }`},
	{name: "8.2/DNF method keeps later parameters", completion: []string{"info"}, src: `
class Svc { function go((Named&Aged)|null $x, Logger $y): void { $y-><|> } }`},
	{name: "8.2/DNF property", completion: []string{"name"}, src: `
class Svc { private (Named&Aged)|null $x = null; function go(): void { $this->x-><|> } }`},
	{name: "8.2/DNF property does not hide later members", completion: []string{"after"}, src: `
class Svc { private (Named&Aged)|null $x = null; public function after(): void { $this-><|> } }`},
	{name: "8.2/DNF return type", completion: []string{"age"}, src: `
class Svc { function get(): (Named&Aged)|null { return null; } function go(): void { $this->get()-><|> } }`},
	{name: "8.2/DNF promoted property", completion: []string{"name"}, src: `
class Svc { function __construct(private (Named&Aged)|null $x) {} function go(): void { $this->x-><|> } }`},
	{name: "8.2/trait constant completes through the class", completion: []string{"LIMIT"}, src: `
trait Limits { public const LIMIT = 10; }
class Svc { use Limits; function go(): void { self::<|> } }`},
	{name: "8.2/trait constant shows in hover", hover: []string{"LIMIT"}, src: `
trait Limits { public const LIMIT = 10; }
class Svc { use Limits; }
function run(): void { echo Svc::LI<|>MIT; }`},
	{name: "8.2/trait constant is known", clean: true, src: `
trait Limits { public const LIMIT = 10; }
class Svc { use Limits; }
function run(): int { return Svc::LIMIT; }`},
	{name: "8.2/readonly class properties show in hover", hover: []string{"readonly"}, src: `
readonly class Svc { public Logger $log; function go(): void { $this->l<|>og; } }`},

	// PHP 8.3
	{name: "8.3/typed constant keeps its name", completion: []string{"NAME"}, src: `
class Svc { const string NAME = 'a'; function go(): void { self::<|> } }`},
	{name: "8.3/typed constant is not named after its type", hover: []string{"NAME", "string"}, src: `
class Svc { const string NAME = 'a'; }
function run(): void { echo Svc::NA<|>ME; }`},
	{name: "8.3/typed constant with a union type", completion: []string{"NAME"}, src: `
class Svc { const int|string NAME = 'a'; function go(): void { self::<|> } }`},
	{name: "8.3/typed constant with a class type flows", hover: []string{"info"}, src: `
class Svc { const Logger LOG = new Logger(); function go(): void { self::LOG->in<|>fo('x'); } }`},
	{name: "8.3/typed interface constant", completion: []string{"NAME"}, src: `
interface Has { const string NAME = 'a'; }
class Svc implements Has { function go(): void { self::<|> } }`},
	{name: "8.3/typed constants are known", clean: true, src: `
class Svc { const string NAME = 'a'; const int|string OTHER = 1; const ?Logger NONE = null; }
function run(): string { return Svc::NAME; }`},
	{name: "8.3/multiple constants in one statement", completion: []string{"A", "B"}, src: `
class Svc { const string A = 'a', B = 'b'; function go(): void { self::<|> } }`},
	{name: "8.3/override with a parent method is clean", clean: true, src: `
class Base { public function go(): void {} }
class Svc extends Base { #[\Override] public function go(): void {} }`},
	{name: "8.3/override with an interface method is clean", clean: true, src: `
class Svc implements Named { #[\Override] public function name(): string { return ''; } }`},
	{name: "8.3/override with a trait abstract method is clean", clean: true, src: `
trait Helps { abstract public function go(): void; }
class Svc { use Helps; #[\Override] public function go(): void {} }`},
	{name: "8.3/override without a parent method is reported", diag: []string{"invalid-override"}, src: `
class Svc { #[\Override] public function go(): void {} }`},
	{name: "8.3/override shows in hover", hover: []string{"#[\\Override]"}, src: `
class Base { public function go(): void {} }
class Svc extends Base { #[\Override] public function go(): void { $this->g<|>o(); } }`},
	{name: "8.3/dynamic constant fetch reads its variable", clean: true, src: `
class Svc { const A = 1; }
function run(string $n): mixed { return Svc::{$n}; }`},
	{name: "8.3/dynamic constant fetch on an enum", clean: true, src: `
enum Suit { case Hearts; }
function run(string $n): mixed { return Suit::{$n}; }`},
	{name: "8.3/dynamic constant fetch variable is checked", diag: []string{"undefined-variable"}, src: `
class Svc { const A = 1; }
function run(): mixed { return Svc::{$nope}; }`},
	{name: "8.3/json_validate returns bool", hover: []string{"bool"}, src: `
function run(string $j): void { json_val<|>idate($j); }`},

	// PHP 8.4
	{name: "8.4/set hook sees its implicit value", clean: true, src: `
class Svc { public string $name { set { $this->name = $value; } } }`},
	{name: "8.4/hooked property completes", completion: []string{"name"}, src: `
class Svc { public string $name { get => 'a'; } function go(): void { $this-><|> } }`},
	{name: "8.4/asymmetric visibility shows in hover", hover: []string{"private(set)"}, src: `
class Svc { public private(set) string $name = ''; function go(): void { $this->na<|>me; } }`},
	{name: "8.4/new without parentheses calls a method", hover: []string{"info"}, src: `
function run(): void { new Logger()->in<|>fo('x'); }`},
	{name: "8.4/new without parentheses completes", completion: []string{"info", "level"}, src: `
function run(): void { new Logger()-><|> }`},
	{name: "8.4/new without parentheses assigns its chain", completion: []string{"info"}, src: `
class Maker { public function log(): Logger { return new Logger(); } }
function run(): void { $l = new Maker()->log(); $l-><|> }`},
	{name: "8.4/new without parentheses reads a constant", hover: []string{"LEVEL"}, src: `
function run(): void { echo new Logger()::LE<|>VEL; }`},
	{name: "8.4/new without parentheses is clean", clean: true, src: `
function run(): int { new Logger()->info('x'); return new Logger()->level; }`},
	{name: "8.4/deprecated attribute on a method is reported", diag: []string{"deprecated"}, src: `
class Svc { #[\Deprecated] public function old(): void {} function go(): void { $this->old(); } }`},
	{name: "8.4/deprecated attribute on a function is reported", diag: []string{"deprecated"}, src: `
#[\Deprecated('use other', since: '1.0')] function old(): void {}
function run(): void { old(); }`},
	{name: "8.4/deprecated attribute shows in hover", hover: []string{"Deprecated", "use other"}, src: `
class Svc { #[\Deprecated('use other')] public function old(): void {} function go(): void { $this->ol<|>d(); } }`},
	{name: "8.4/array_find has a signature", hover: []string{"callable", "mixed"}, src: `
function run(array $a): void { array_fi<|>nd($a, fn ($x) => true); }`},
	{name: "8.4/array_find_key has a signature", hover: []string{"callable"}, src: `
function run(array $a): void { array_find_k<|>ey($a, fn ($x) => true); }`},
	{name: "8.4/array_any returns bool", hover: []string{"bool"}, src: `
function run(array $a): void { array_a<|>ny($a, fn ($x) => true); }`},
	{name: "8.4/array_all returns bool", hover: []string{"bool"}, src: `
function run(array $a): void { array_a<|>ll($a, fn ($x) => true); }`},
	{name: "8.4/deprecated attribute on a constant is reported", diag: []string{"deprecated"}, src: `
class Svc { #[\Deprecated] const OLD = 1; }
function run(): int { return Svc::OLD; }`},
	{name: "8.4/deprecated attribute on an enum case is reported", diag: []string{"deprecated"}, src: `
enum Suit { #[\Deprecated] case Old; }
function run(): Suit { return Suit::Old; }`},
	{name: "8.4/lazy object factories complete", completion: []string{"getName", "newLazyGhost", "newLazyProxy", "isUninitializedLazyObject", "initializeLazyObject", "markLazyObjectAsInitialized", "resetAsLazyGhost", "resetAsLazyProxy"}, src: `
function run(\ReflectionClass $r): void { $r-><|> }`},
	{name: "8.4/lazy ghost returns an object", hover: []string{"object"}, src: `
function run(\ReflectionClass $r): void { $r->newLazyGh<|>ost(fn ($o) => null); }`},
	{name: "8.4/lazy property helpers complete", completion: []string{"setRawValueWithoutLazyInitialization", "skipLazyInitialization", "isLazy"}, src: `
function run(\ReflectionProperty $r): void { $r-><|> }`},

	// PHP 8.5
	{name: "8.5/pipe into a first-class callable types the result", completion: []string{"info"}, src: `
function make(string $s): Logger { return new Logger(); }
function run(): void { $l = 'a' |> make(...); $l-><|> }`},
	{name: "8.5/pipe chain uses the last stage", completion: []string{"name"}, src: `
function make(string $s): Logger { return new Logger(); }
function other(Logger $l): Person { return new Person(); }
function run(): void { $p = 'a' |> make(...) |> other(...); $p-><|> }`},
	{name: "8.5/pipe into a typed closure", completion: []string{"info"}, src: `
function run(): void { $l = 'a' |> fn (string $s): Logger => new Logger(); $l-><|> }`},
	{name: "8.5/pipe into a method callable", completion: []string{"info"}, src: `
class Svc { function make(string $s): Logger { return new Logger(); }
function go(): void { $l = 'a' |> $this->make(...); $l-><|> } }`},
	{name: "8.5/pipe into a static callable", completion: []string{"info"}, src: `
class Svc { static function make(string $s): Logger { return new Logger(); } }
function run(): void { $l = 'a' |> Svc::make(...); $l-><|> }`},
	{name: "8.5/pipe is clean", clean: true, src: `
function run(string $s): int { return $s |> trim(...) |> strlen(...); }`},
	{name: "8.5/pipe reads the variable", diag: []string{"undefined-variable"}, src: `
function run(): int { return $nope |> strlen(...); }`},
	{name: "8.5/discarded NoDiscard function is reported", diag: []string{"unused-result"}, src: `
#[\NoDiscard] function must(): int { return 1; }
function run(): void { must(); }`},
	{name: "8.5/discarded NoDiscard method is reported", diag: []string{"unused-result"}, src: `
class Svc { #[\NoDiscard('check it')] public function must(): int { return 1; } function go(): void { $this->must(); } }`},
	{name: "8.5/used NoDiscard result is clean", clean: true, src: `
#[\NoDiscard] function must(): int { return 1; }
function run(): int { $x = must(); (void) must(); if (must()) { echo $x; } return must(); }`},
	{name: "8.5/NoDiscard shows in hover", hover: []string{"NoDiscard"}, src: `
#[\NoDiscard] function must(): int { return 1; }
function run(): int { return mu<|>st(); }`},
	{name: "8.5/clone with keeps the type", completion: []string{"info"}, src: `
function run(Logger $a): void { $b = clone($a, ['level' => 1]); $b-><|> }`},
	{name: "8.5/clone without with keeps the type", completion: []string{"info"}, src: `
function run(Logger $a): void { $b = clone $a; $b-><|> }`},
	{name: "8.5/clone with parentheses keeps the type", completion: []string{"info"}, src: `
function run(Logger $a): void { $b = clone($a); $b-><|> }`},
	{name: "8.5/clone with is clean", clean: true, src: `
function run(Logger $a, int $n): Logger { return clone($a, ['level' => $n]); }`},
	{name: "8.5/clone with reads its arguments", diag: []string{"undefined-variable"}, src: `
function run(Logger $a): Logger { return clone($a, ['level' => $nope]); }`},
}

func TestPHPVersionFeatures(t *testing.T) {
	for _, tc := range phpVersionCases {
		t.Run(tc.name, func(t *testing.T) {
			runPHPVersionCase(t, tc)
		})
	}
}

func runPHPVersionCase(t *testing.T, tc phpVersionCase) {
	t.Helper()
	root := t.TempDir()
	// Cases are written compactly but laid out over several lines, as real code
	// is: much of the server reads the source line by line.
	src := strings.NewReplacer("{ ", "{\n", "; ", ";\n", " }", "\n}").Replace(phpVersionPrelude + tc.src)
	line, char := -1, -1
	if i := strings.Index(src, "<|>"); i >= 0 {
		before := src[:i]
		line = strings.Count(before, "\n")
		char = i - (strings.LastIndex(before, "\n") + 1)
		src = before + src[i+len("<|>"):]
	}
	path := filepath.Join(root, "case.php")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	composer := `{"require": {"php": "^8.5", "ext-json": "*"}}`
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(composer), 0o644); err != nil {
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

	position := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": char},
	}
	if len(tc.hover) > 0 {
		got := responseText(h.readResponse(h.send("textDocument/hover", position)))
		for _, want := range tc.hover {
			if !strings.Contains(got, want) {
				t.Errorf("hover missing %q in:\n%s", want, got)
			}
		}
	}
	if len(tc.completion) > 0 {
		got := responseText(h.readResponse(h.send("textDocument/completion", position)))
		for _, want := range tc.completion {
			if !strings.Contains(got, `"label":"`+want) {
				t.Errorf("completion missing %q", want)
			}
		}
	}
	if tc.definition {
		resp := h.readResponse(h.send("textDocument/definition", position))
		if result := resp["result"]; result == nil || responseText(resp) == `{"result":[]}` {
			t.Errorf("definition found nothing: %v", resp)
		}
	}
	if tc.clean || len(tc.diag) > 0 || len(tc.noDiag) > 0 {
		diags := collectDiagnostics(h, uri)
		if tc.clean && len(diags) > 0 {
			t.Errorf("expected no diagnostics, got %v", diags)
		}
		for _, code := range tc.noDiag {
			if hasDiagnostic(diags, code) {
				t.Errorf("unexpected %s in %v", code, diags)
			}
		}
		for _, code := range tc.diag {
			if !hasDiagnostic(diags, code) {
				t.Errorf("missing %s in %v", code, diags)
			}
		}
	}
}

func responseText(resp map[string]interface{}) string {
	body, _ := json.Marshal(resp)
	return strings.ReplaceAll(string(body), `\\`, `\`)
}

// collectDiagnostics returns "code: message" for the latest publish on uri,
// waiting out the gap between the fast and slow diagnostic passes.
func collectDiagnostics(h *lspHarness, uri string) []string {
	h.t.Helper()
	var latest []string
	deadline := time.After(1500 * time.Millisecond)
	for {
		select {
		case msg := <-h.notes:
			if method, _ := msg["method"].(string); method != "textDocument/publishDiagnostics" {
				continue
			}
			params, _ := msg["params"].(map[string]interface{})
			if params["uri"] != uri {
				continue
			}
			latest = latest[:0]
			items, _ := params["diagnostics"].([]interface{})
			for _, item := range items {
				d, _ := item.(map[string]interface{})
				code, _ := d["code"].(string)
				message, _ := d["message"].(string)
				latest = append(latest, code+": "+message)
			}
		case <-deadline:
			return latest
		}
	}
}

func hasDiagnostic(diags []string, code string) bool {
	for _, d := range diags {
		if strings.HasPrefix(d, code+":") {
			return true
		}
	}
	return false
}
