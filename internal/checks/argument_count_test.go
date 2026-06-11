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

	t.Run("too few arguments not flagged (conservative - cannot detect optional params)", func(t *testing.T) {
		// The parser does not currently propagate HasDefault through ParamNode→ParamInfo,
		// so we cannot reliably distinguish required vs optional params.
		// The "too few" check is conservatively disabled to avoid false positives.
		idx := indexWithFunctions(`<?php
function add(int $a, int $b): int {
    return $a + $b;
}
`)
		source := `<?php
$result = add(1);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		// Conservatively not flagged
		assertNoFindings(t, findings)
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

	t.Run("static method too many args flagged", func(t *testing.T) {
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
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		assertFindingCodes(t, findings, []string{"argument-count-mismatch"})
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
