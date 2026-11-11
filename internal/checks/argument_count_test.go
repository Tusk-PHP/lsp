package checks

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// indexWithFunctions creates a symbol index with the given PHP source pre-indexed.
func indexWithFunctions(php string) *symbols.Index {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///helpers.php", php)
	return idx
}

func TestArgumentCountRule(t *testing.T) {
	rule := &ArgumentCountRule{}

	t.Run("trailing comma is not an extra argument", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
function env(string $key, mixed $default = null): mixed {}
`)
		source := `<?php
$a = env(
    'HORIZON_PREFIX',
    strtolower('x') . '_horizon:',
);
`
		file := parser.ParseFile(source)
		assertNoFindings(t, rule.Check(file, source, idx))
	})

	t.Run("declarations sharing a global function's name are not calls", func(t *testing.T) {
		// WordPress defines a global init() in rss.php; WP_User::init() takes two
		// parameters and must not be checked against it.
		idx := indexWithFunctions(`<?php
function init() {}
function __construct() {}
class Item {}
`)
		source := `<?php
class WP_User {
    public function init( $data, $site_id = 0 ) {}
    public function __construct( $id = 0, $name = '', $site_id = '' ) {}
    public static function &init_ref( $a, $b ) {}
}
function &init( $a, $b ) {}
$item = new Item( 1, 2, 3 );
`
		file := parser.ParseFile(source)
		assertNoFindings(t, rule.Check(file, source, idx))

		// A real call to the global function is still checked.
		call := "<?php\ninit( 1, 2 );\n"
		if got := rule.Check(parser.ParseFile(call), call, idx); len(got) != 1 {
			t.Errorf("expected the real init() call to be flagged, got %+v", got)
		}
	})

	t.Run("correct arg count not flagged", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
function greet(string $name): string {
    return "Hello $name";
}
`)
		source := `<?php
$msg = greet("Alice");
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertNoFindings(t, findings)
	})

	t.Run("too few arguments flagged", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
function add(int $a, int $b): int {
    return $a + $b;
}
function pad(string $s, int $n = 2, string $c = ' '): string { return $s; }
`)
		source := `<?php
$result = add(1);
$ok = pad('x');
$ok = pad('x', 3, '-');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertFindingCodes(t, findings, []string{"argument-count-mismatch"})
		if len(findings) == 1 && findings[0].Message != "Too few arguments to add(): 1 passed, at least 2 required" {
			t.Errorf("message: %q", findings[0].Message)
		}
	})

	t.Run("too many arguments flagged", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
function greet(string $name): string {
    return "Hello $name";
}
`)
		source := `<?php
$msg = greet("Alice", "extra");
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertFindingCodes(t, findings, []string{"argument-count-mismatch"})
	})

	t.Run("zero args for zero-param function not flagged", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
function doNothing(): void {}
`)
		source := `<?php
doNothing();
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertNoFindings(t, findings)
	})

	t.Run("optional param allows fewer args", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
function greet(string $name, string $greeting = "Hello"): string {
    return "$greeting $name";
}
`)
		source := `<?php
$msg = greet("Alice");
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertNoFindings(t, findings)
	})

	t.Run("variadic function allows any number of args", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
function sum(int ...$nums): int {
    return array_sum($nums);
}
`)
		source := `<?php
$s = sum(1, 2, 3, 4, 5);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertNoFindings(t, findings)
	})

	t.Run("spread argument skips check", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
function add(int $a, int $b): int {
    return $a + $b;
}
`)
		source := `<?php
$args = [1, 2];
$result = add(...$args);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertNoFindings(t, findings)
	})

	t.Run("named argument syntax skips check", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
function add(int $a, int $b): int {
    return $a + $b;
}
`)
		source := `<?php
$result = add(a: 1, b: 2);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertNoFindings(t, findings)
	})

	t.Run("nil index returns nil", func(t *testing.T) {
		source := `<?php
doSomething(1, 2, 3);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("nil file returns nil", func(t *testing.T) {
		idx := indexWithFunctions(`<?php function f(): void {}`)
		findings := rule.Check(nil, "", idx)
		assertNoFindings(t, findings)
	})

	// PHP allows extra arguments, and libraries read them with
	// func_get_args() to stay compatible, as Ulid::isValid($id, $format) does.
	t.Run("static method too many args not flagged, too few flagged", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
namespace App;
class Math {
    public static function multiply(int $a, int $b): int {
        return $a * $b;
    }
}
`)
		source := `<?php
namespace App;
use App\Math;
$result = Math::multiply(1, 2, 3, 4);
$result = Math::multiply(1);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertFindingCodes(t, findings, []string{"argument-count-mismatch"})
		if len(findings) == 1 && findings[0].StartLine != 4 {
			t.Errorf("finding on line %d, want 4", findings[0].StartLine)
		}
	})

	t.Run("class with __call skips check", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
namespace App;
class Magic {
    public function doThing(int $x): void {}
    public function __call(string $name, array $args): mixed { return null; }
}
`)
		source := `<?php
namespace App;
use App\Magic;
Magic::doThing(1, 2, 3);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		// __call is defined, so check is skipped
		assertNoFindings(t, findings)
	})

	t.Run("function not in index not flagged", func(t *testing.T) {
		idx := symbols.NewIndex()
		source := `<?php
$result = unknownFunction(1, 2, 3);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertNoFindings(t, findings)
	})

	t.Run("method call via arrow not checked (no type resolution)", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
namespace App;
class Greeter {
    public function greet(string $name): string {
        return "Hello $name";
    }
}
`)
		source := `<?php
$greeter = new App\Greeter();
$greeter->greet("Alice", "extra"); // extra arg via arrow, but we cannot check
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		// Arrow method calls are skipped (we can't resolve $greeter's type)
		assertNoFindings(t, findings)
	})

	t.Run("empty call to parameterless function not flagged", func(t *testing.T) {
		idx := indexWithFunctions(`<?php
function init(): void {}
`)
		source := `<?php
init();
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertNoFindings(t, findings)
	})
}

// A call through -> is checked against the method of the receiver's class,
// when that's one known class; @method tags, __call and unions aren't. Only
// too few arguments is reported there: PHP allows too many, and libraries
// read them with func_get_args().
func TestArgumentCountOnMethodCalls(t *testing.T) {
	idx := indexWithFunctions(`<?php
namespace App;
class Mailer {
    public function send(string $to, string $subject): bool { return true; }
    public function queue(string $to, int $delay = 0): void {}
}
class Other { public function send(): void {} }
/** @method void magic(string $a, string $b) */
class Magic { public function __call($n, $a) {} }
/** @method void tagged(string $a, string $b) */
class Tagged {}
`)
	types := map[string]string{"$this": `App\Mailer`, "$mailer": `App\Mailer`, "$either": `App\Mailer|App\Other`, "$magic": `App\Magic`, "$tagged": `App\Tagged`}
	rule := &ArgumentCountRule{TypeResolver: func(expr, _ string, _, _ int, _ *parser.FileNode) string { return types[expr] }}
	source := `<?php
namespace App;
$mailer->send('a@example.test');
$mailer?->send('a', 'b', 'c');
$mailer->queue('a');
$mailer->send('a', 'b');
$either->send('a');
$magic->magic('a');
$tagged->tagged('a');
$unknown->send('a');
$f = function () { $this->send('a'); };
$mailer->send(...$args);
$mailer->send(to: 'a', subject: 'b');
`
	got := rule.Check(parser.ParseFile(source), source, idx)
	var messages []string
	for _, f := range got {
		messages = append(messages, f.Message)
	}
	want := []string{
		"Too few arguments to send(): 1 passed, at least 2 required",
	}
	if len(messages) != len(want) || messages[0] != want[0] {
		t.Errorf("got %q, want %q", messages, want)
	}
	if len(got) > 0 && (got[0].StartLine != 2 || got[0].StartCol != 9) {
		t.Errorf("first finding at %d:%d, want 2:9", got[0].StartLine, got[0].StartCol)
	}
}

// Code that checks a version first is written for several versions of PHP or a
// library, whose signatures differ. Laravel calls PHPUnit's ErrorHandler
// enable() with or without an argument depending on PHPUnit's version.
func TestArgumentCountSkipsVersionGatedBranches(t *testing.T) {
	idx := indexWithFunctions(`<?php
namespace App;
class Handler { public function enable(string $test): void {} }
class Kernel { const VERSION_ID = 70000; }
function pair($a, $b) {}
`)
	types := map[string]string{"$instance": `App\Handler`}
	rule := &ArgumentCountRule{TypeResolver: func(expr, _ string, _, _ int, _ *parser.FileNode) string { return types[expr] }}
	source := `<?php
namespace App;
if (version_compare(Version::id(), '12.3.4', '>=')) {
    $instance->enable($test);
} else {
    $instance->enable();
}
if ($legacy) {
    pair(1);
} elseif (\PHP_VERSION_ID < 80100) {
    pair(1);
} else if ($other) {
    pair(1);
}
if (Kernel::VERSION_ID >= 70000) pair(1);
if (PHP_VERSION >= '8.0'):
    pair(1);
endif;
$instance->enable();
if ($unrelated) {
    pair(1);
}
`
	got := rule.Check(parser.ParseFile(source), source, idx)
	var lines []int
	for _, f := range got {
		lines = append(lines, f.StartLine)
	}
	if len(lines) != 2 || lines[0] != 18 || lines[1] != 20 {
		t.Errorf("findings on lines %v, want [18 20] (only outside the version checks)", lines)
	}
}
