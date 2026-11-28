package checks

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

type compileErrorFrom struct {
	from, to token.Version
	message  string
}

func since(v token.Version, message string) []compileErrorFrom {
	return []compileErrorFrom{{v, token.Latest, message}}
}

func always(message string) []compileErrorFrom {
	return since(token.PHP74, message)
}

const (
	nestedTernary = "Unparenthesized `a ? b : c ? d : e` is not supported. Use either `(a ? b : c) ? d : e` or `a ? b : (c ? d : e)`"
	ternaryShort  = "Unparenthesized `a ? b : c ?: d` is not supported. Use either `(a ? b : c) ?: d` or `a ? b : (c ?: d)`"
	shortTernary  = "Unparenthesized `a ?: b ? c : d` is not supported. Use either `(a ?: b) ? c : d` or `a ?: (b ? c : d)`"
	curlyOffset   = "Array and string offset access syntax with curly braces is no longer supported"
	firstStrict   = "strict_types declaration must be the very first statement in the script"
	combineNamed  = "Cannot combine named arguments and argument unpacking"
)

var compileErrorCases = []struct {
	name string
	src  string
	want []compileErrorFrom
}{
	{"nested ternary", `$x = 1 ? 2 : 3 ? 4 : 5;`, since(token.PHP80, nestedTernary)},
	{"nested ternary inside parentheses", `$x = (1 ? 2 : 3 ? 4 : 5);`, since(token.PHP80, nestedTernary)},
	{"parenthesized first operand", `$x = (1) ? 2 : 3 ? 4 : 5;`, since(token.PHP80, nestedTernary)},
	{"ternary chain reports once", `$x = 1 ? 2 : 3 ? 4 : 5 ?: 6;`, since(token.PHP80, ternaryShort)},
	{"ternary then short ternary", `$x = 1 ? 2 : 3 ?: 4;`, since(token.PHP80, ternaryShort)},
	{"short ternary then ternary", `$x = 1 ?: 2 ? 3 : 4;`, since(token.PHP80, shortTernary)},
	{"short ternary chain", `$x = 1 ?: 2 ?: 3;`, nil},
	{"parenthesized ternaries", `$x = (1 ? 2 : 3) ? 4 : 5; $y = 1 ? 2 : (3 ? 4 : 5); $z = 1 ? 2 ? 3 : 4 : 5;`, nil},

	{"curly offset", `$a = "x"; echo $a{0};`, []compileErrorFrom{{token.PHP80, token.PHP83, curlyOffset}}},
	{"curly offset in interpolation", `$a = "x"; echo "{$a{0}}";`, []compileErrorFrom{{token.PHP80, token.PHP83, curlyOffset}}},
	{"square offset", `$a = "x"; echo $a[0];`, nil},

	{"unset cast", `$a = 1; var_dump((unset) $a);`, since(token.PHP80, "The (unset) cast is no longer supported")},

	{"hooks on a static property", `class A { public static int $p { get => 1; } }`, since(token.PHP84, "Cannot declare hooks for static property")},
	{"hooks on a static interface property", `interface I { public static int $p { get; } }`, since(token.PHP84, "Cannot declare hooks for static property")},
	{"redeclared hook", `class A { public int $p { get => 1; GET => 2; } }`, since(token.PHP84, `Cannot redeclare property hook "GET"`)},
	{"redeclared promoted hook", `class A { public function __construct(public int $p { get => 1; get => 2; }) {} }`, since(token.PHP84, `Cannot redeclare property hook "get"`)},
	{"unknown hook", `class A { public int $p { Foo => 1; } }`, since(token.PHP84, `Unknown hook "Foo" for property A::$p, expected "get" or "set"`)},
	{"unknown promoted hook", `namespace N; class A { public function __construct(public int $p { foo => 1; }) {} }`, since(token.PHP84, `Unknown hook "foo" for property N\A::$p, expected "get" or "set"`)},
	{"hooks", `class A { public int $p { Get => 1; set => null; } }`, nil},

	{"break 0", `while (1) { break 0; }`, always("'break' operator accepts only positive integers")},
	{"continue 0", `while (1) { continue 0; }`, always("'continue' operator accepts only positive integers")},
	{"break with a string", `while (1) { break "a"; }`, always("'break' operator accepts only positive integers")},
	{"break with a variable", `$n = 1; while (1) { break $n; }`, always("'break' operator with non-integer operand is no longer supported")},
	{"break with a negative number", `while (1) { break -1; }`, always("'break' operator with non-integer operand is no longer supported")},
	{"break with a constant", `while (1) { break FOO; }`, always("'break' operator with non-integer operand is no longer supported")},
	{"break with a magic constant", `while (1) { break __LINE__; }`, always("'break' operator with non-integer operand is no longer supported")},
	{"break with an interpolated string", `while (1) { break "$x"; }`, always("'break' operator with non-integer operand is no longer supported")},
	{"break with a shell command", "while (1) { break `ls`; }", always("'break' operator with non-integer operand is no longer supported")},
	{"break with a heredoc", "while (1) { break <<<EOT\n1\nEOT;\n}", always("'break' operator accepts only positive integers")},
	{"break with a nowdoc", "while (1) { break <<<'EOT'\n1\nEOT;\n}", always("'break' operator accepts only positive integers")},
	{"break with an interpolating heredoc", "while (1) { break <<<EOT\n$x\nEOT;\n}", always("'break' operator with non-integer operand is no longer supported")},
	{"break outside a loop", `break;`, always("'break' not in the 'loop' or 'switch' context")},
	{"continue outside a loop", `continue;`, always("'continue' not in the 'loop' or 'switch' context")},
	{"break in a method", `class A { function f() { break; } }`, always("'break' not in the 'loop' or 'switch' context")},
	{"break in a closure in a loop", `while (1) { $f = function () { break; }; }`, always("'break' not in the 'loop' or 'switch' context")},
	{"break too many levels", `while (1) { break 2; }`, always("Cannot 'break' 2 levels")},
	{"break levels in a closure", `while (1) { $f = function () { while (1) { break 2; } }; }`, always("Cannot 'break' 2 levels")},
	{"continue too many levels", `while (1) { continue 0x2; }`, always("Cannot 'continue' 2 levels")},
	{"break levels with separators", `while (1) { while (1) { break 1_1; } }`, always("Cannot 'break' 11 levels")},
	{"breaks", `while (1) { switch (1) { case 1: break 2; } } foreach ([] as $x) { while (1): break 2; endwhile; } do { break (1); } while (0); for (;;) { continue 0b1; }`, nil},

	{"$this parameter", `function f($this) {}`, always("Cannot use $this as parameter")},
	{"$this arrow function parameter", `fn($this) => 1;`, always("Cannot use $this as parameter")},
	{"$this promoted parameter", `class A { function __construct(public $this) {} }`, since(token.PHP80, "Cannot use $this as parameter")},
	{"auto-global parameter", `function f($_GET) {}`, always("Cannot re-assign auto-global variable _GET")},
	{"duplicate parameter", `function f($a, $a) {}`, always("Redefinition of parameter $a")},
	{"duplicate arrow function parameter", `fn($a, $a) => 1;`, always("Redefinition of parameter $a")},
	{"variadic before another parameter", `function f($a, ...$b, $c) {}`, always("Only the last parameter can be variadic")},
	{"variadic with a default", `function f(...$a = []) {}`, always("Variadic parameter cannot have a default value")},
	{"parameters differing in case", `function f($A, $a, ...$b) {}`, nil},
	{"$this in use", `$f = function () use ($this) {};`, always("Cannot use $this as lexical variable")},
	{"auto-global in use", `$f = function () use ($GLOBALS) {};`, always("Cannot use auto-global as lexical variable")},
	{"use twice", `$f = function () use ($a, $a) {};`, always("Cannot use variable $a twice")},
	{"use of a parameter", `$f = function ($a) use (&$a) {};`, always("Cannot use lexical variable $a as a parameter name")},

	{"assign to $this", `class A { function f() { $this = 1; } }`, always("Cannot re-assign $this")},
	{"assign a reference to $this", `$this = &$x;`, always("Cannot re-assign $this")},
	{"coalesce-assign to $this", `$this ??= 1;`, always("Cannot re-assign $this")},
	{"destructure into $this", `[$a, [$this]] = [1, [2]];`, always("Cannot re-assign $this")},
	{"list into $this", `list("k" => $this) = [];`, always("Cannot re-assign $this")},
	{"foreach into $this", `foreach ([] as $this => $v) {}`, always("Cannot re-assign $this")},
	{"foreach destructures into $this", `foreach ([] as [$this]) {}`, always("Cannot re-assign $this")},
	{"catch into $this", `try {} catch (Exception $this) {}`, always("Cannot re-assign $this")},
	{"catch without a variable", `try {} catch (Exception) {}`, nil},
	{"unset $this", `unset($a, $this);`, always("Cannot unset $this")},
	{"global $this", `function f() { global $a, $this; }`, always("Cannot use $this as global variable")},
	{"static $this", `static $a = 1, $this;`, always("Cannot use $this as static variable")},
	{"$this uses", `$this .= 1; $this++; $r = &$this; unset($this->x, $this[0]); isset($this); $this->a = 1;`, nil},

	{"abstract method with a body", `abstract class A { abstract function f() {} }`, always("Abstract function A::f() cannot contain body")},
	{"abstract trait method with a body", `trait T { abstract function f() {} }`, always("Abstract function T::f() cannot contain body")},
	{"interface method with a body", `interface I { function f() {} }`, always("Interface function I::f() cannot contain body")},
	{"method without a body", `class A { function f(); }`, always("Non-abstract method A::f() must contain body")},
	{"trait method without a body", `trait T { function f(); }`, always("Non-abstract method T::f() must contain body")},
	{"enum method without a body", `enum E { function f(); }`, since(token.PHP81, "Non-abstract method E::f() must contain body")},
	{"private interface method", `interface I { private function f(); }`, []compileErrorFrom{
		{token.PHP74, token.PHP80, "Access type for interface method I::f() must be omitted"},
		{token.PHP81, token.Latest, "Access type for interface method I::f() must be public"},
	}},
	{"abstract interface method", `interface I { abstract function f(); }`, []compileErrorFrom{
		{token.PHP74, token.PHP80, "Access type for interface method I::f() must be omitted"},
		{token.PHP81, token.Latest, "Interface method I::f() must not be abstract"},
	}},
	{"final interface method", `interface I { final function f(); }`, []compileErrorFrom{
		{token.PHP74, token.PHP80, "Access type for interface method I::f() must be omitted"},
		{token.PHP81, token.Latest, "Interface method I::f() must not be final"},
	}},
	{"methods", `interface I { public static function f(); } abstract class A { abstract function f(); function g() {} }`, nil},

	{"static constructor", `class A { static function __CONSTRUCT() {} }`, []compileErrorFrom{
		{token.PHP74, token.PHP74, "Constructor A::__CONSTRUCT() cannot be static"},
		{token.PHP80, token.Latest, "Method A::__CONSTRUCT() cannot be static"},
	}},
	{"static destructor", `class A { public static function __destruct() {} }`, []compileErrorFrom{
		{token.PHP74, token.PHP74, "Destructor A::__destruct() cannot be static"},
		{token.PHP80, token.Latest, "Method A::__destruct() cannot be static"},
	}},
	{"static clone", `trait T { public static function __clone() {} }`, []compileErrorFrom{
		{token.PHP74, token.PHP74, "Clone method T::__clone() cannot be static"},
		{token.PHP80, token.Latest, "Method T::__clone() cannot be static"},
	}},
	{"static interface constructor", `interface I { static function __construct(); }`, since(token.PHP80, "Method I::__construct() cannot be static")},
	{"constructor return type", `class A { function __construct(): void {} }`, []compileErrorFrom{
		{token.PHP74, token.PHP74, "Constructor A::__construct() cannot declare a return type"},
		{token.PHP80, token.Latest, "Method A::__construct() cannot declare a return type"},
	}},
	{"destructor return type", `class A { function __destruct(): void {} }`, []compileErrorFrom{
		{token.PHP74, token.PHP74, "Destructor A::__destruct() cannot declare a return type"},
		{token.PHP80, token.Latest, "Method A::__destruct() cannot declare a return type"},
	}},
	{"interface constructor return type", `interface I { function __construct(): void; }`, since(token.PHP80, "Method I::__construct() cannot declare a return type")},

	{"redeclared method", `class A { function f() {} function F() {} }`, always("Cannot redeclare A::F()")},
	{"redeclared method in the global namespace block", `namespace { class A { function f() {} function f() {} } }`, always("Cannot redeclare A::f()")},
	{"redeclared method in a namespace", `namespace N; class A { function f() {} function f() {} }`, always(`Cannot redeclare N\A::f()`)},
	{"redeclared property", `class A { public $p, $p; }`, always("Cannot redeclare A::$p")},
	{"property redeclaring a promoted one", `class A { public function __construct(public $p) {} public $p; }`, since(token.PHP80, "Cannot redeclare A::$p")},
	{"duplicate promoted parameter", `class A { public function __construct(public $p, public $p) {} }`, since(token.PHP80, "Redefinition of parameter $p")},
	{"redefined constant", `interface I { const C = 1; const C = 2; }`, always("Cannot redefine class constant I::C")},
	{"enum case redefining a constant", `enum E { case A; const A = 1; }`, since(token.PHP81, "Cannot redefine class constant E::A")},
	{"members sharing a name", `class A { const C = 1; public $C; function C() {} }`, nil},
	{"anonymous class", `$x = new class { function f() {} function f() {} };`, nil},

	{"value of a pure enum case", `enum E { case A = "x"; }`, []compileErrorFrom{
		{token.PHP81, token.PHP81, `Case A of non-backed enum E must not have a value, try adding ": string" to the enum declaration`},
		{token.PHP82, token.Latest, "Case A of non-backed enum E must not have a value"},
	}},
	{"integer value of a pure enum case", `enum E { case A = 1; }`, []compileErrorFrom{
		{token.PHP81, token.PHP81, `Case A of non-backed enum E must not have a value, try adding ": int" to the enum declaration`},
		{token.PHP82, token.Latest, "Case A of non-backed enum E must not have a value"},
	}},
	{"float value of a pure enum case", `enum E { case A = 1.5; }`, since(token.PHP81, "Case A of non-backed enum E must not have a value")},
	{"standalone null, false and true return types", `function f(): null { return; } function g(): false { return; } function h(): true { return; }`, nil},
	{"class return types outside a class", `function f(): static { return; } function g(): self { return; } function h(): parent { return; }`, nil},
	{"case in a class", `class A { case X; }`, since(token.PHP81, "Case can only be used in enums")},
	{"case in an interface", `interface I { case X; }`, since(token.PHP81, "Case can only be used in enums")},
	{"case in a trait", `trait T { case X = 1; }`, since(token.PHP81, "Case can only be used in enums")},
	{"case in an anonymous class", `new class { case X; };`, since(token.PHP81, "Case can only be used in enums")},
	{"constant value of a pure enum case", `enum E { case A = E::B; const B = 1; }`, []compileErrorFrom{
		{token.PHP82, token.Latest, "Case A of non-backed enum E must not have a value"},
	}},
	{"backed enum case without a value", `enum E: int { case A; }`, since(token.PHP81, "Case A of backed enum E must have a value")},
	{"enum property", `enum E { case A; public static $p; }`, []compileErrorFrom{
		{token.PHP81, token.PHP81, "Enums may not include properties"},
		{token.PHP82, token.Latest, "Enum E cannot include properties"},
	}},
	{"enums", `enum E: string { case A = "a"; const B = self::A; } enum F { case A; }`, nil},

	{"void function returns a value", `function f(): VOID { return 1; }`, always("A void function must not return a value")},
	{"void function returns null", `function f(): void { return \null; }`, always(`A void function must not return a value (did you mean "return;" instead of "return null;"?)`)},
	{"void method returns a value", `trait T { static function f(): void { return 1; } }`, []compileErrorFrom{
		{token.PHP74, token.PHP83, "A void function must not return a value"},
		{token.PHP84, token.Latest, "A void method must not return a value"},
	}},
	{"void closure returns a value", `$f = function (): void { return 1; };`, always("A void function must not return a value")},
	{"void closure in a class returns a value", `class A { function f() { $f = static function (): void { return 1; }; } }`, []compileErrorFrom{
		{token.PHP74, token.PHP83, "A void function must not return a value"},
		{token.PHP84, token.Latest, "A void method must not return a value"},
	}},
	{"typed closure in an arrow function in a class returns nothing", `class A { function f() { $f = fn() => function (): int { return; }; } }`, []compileErrorFrom{
		{token.PHP74, token.PHP83, "A function with return type must return a value"},
		{token.PHP84, token.Latest, "A method with return type must return a value"},
	}},
	{"void function in a method returns a value", `class A { function f() { function g(): void { return 1; } } }`, always("A void function must not return a value")},
	{"never function returns", `class A { function f(): never { return; } }`, []compileErrorFrom{
		{token.PHP74, token.PHP80, "A function with return type must return a value"},
		{token.PHP81, token.PHP83, "A never-returning function must not return"},
		{token.PHP84, token.Latest, "A never-returning method must not return"},
	}},
	{"typed function returns nothing", `function f(): int { $g = fn() => yield 1; return; }`, always("A function with return type must return a value")},
	{"nullable function returns nothing", `function f(): ?int { return; }`, always(`A function with return type must return a value (did you mean "return null;" instead of "return;"?)`)},
	{"union with null returns nothing", `function f(): null|int { return; }`, since(token.PHP80, `A function with return type must return a value (did you mean "return null;" instead of "return;"?)`)},
	{"mixed function returns nothing", `function f(): mixed { return; }`, []compileErrorFrom{
		{token.PHP74, token.PHP74, "A function with return type must return a value"},
		{token.PHP80, token.Latest, `A function with return type must return a value (did you mean "return null;" instead of "return;"?)`},
	}},
	{"static method returns nothing", `class A { function f(): static { return; } }`, []compileErrorFrom{
		{token.PHP80, token.PHP83, "A function with return type must return a value"},
		{token.PHP84, token.Latest, "A method with return type must return a value"},
	}},
	{"returns", `function f(): void { return; $g = function () { return 1; }; } function g(): Generator { yield 1; return; } function i(): ?int { return null; }`, nil},

	{"positional after named", `f(a: 1, 2);`, since(token.PHP80, "Cannot use positional argument after named argument")},
	{"positional after named in a method call", `$o->m(a: 1, 2);`, since(token.PHP80, "Cannot use positional argument after named argument")},
	{"positional after named in a static call", `A::m(a: 1, 2);`, since(token.PHP80, "Cannot use positional argument after named argument")},
	{"positional after named in new", `new class(a: 1, 2) {};`, since(token.PHP80, "Cannot use positional argument after named argument")},
	{"positional after named in an attribute", `#[Attr(a: 1, 2)] function g() {}`, since(token.PHP80, "Cannot use positional argument after named argument")},
	{"placeholder after named", `echo f(a: 1, ?);`, since(token.PHP86, "Cannot use positional argument after named argument")},
	{"unpacking after named", `f(a: 1, ...[]);`, []compileErrorFrom{
		{token.PHP80, token.PHP80, combineNamed},
		{token.PHP81, token.Latest, "Cannot use argument unpacking after named arguments"},
	}},
	{"named after unpacking", `f(...$x, a: 1);`, []compileErrorFrom{{token.PHP80, token.PHP80, combineNamed}}},
	{"positional after unpacking", `f(...$a, 1);`, always("Cannot use positional argument after argument unpacking")},
	{"arguments", `f(1, a: 2, b: 3); f(...$a, ...$b); f(...);`, nil},

	{"goto an undefined label", `function f() { goto x; } x: ;`, always("'goto' to undefined label 'x'")},
	{"goto a label in another case", `goto X; x: ;`, always("'goto' to undefined label 'X'")},
	{"goto a label in a closure", `$f = function () { goto x; }; x: ;`, always("'goto' to undefined label 'x'")},
	{"label defined twice", `x: x: ;`, always("Label 'x' already defined")},
	{"labels", `x: function f() { x: ; goto x; } goto x;`, nil},

	{"strict_types after a statement", "$x = 1; declare(strict_types=1);", always(firstStrict)},
	{"strict_types after a namespace", "namespace A;\ndeclare(strict_types=1);", always(firstStrict)},
	{"strict_types in a braced namespace", "namespace A { declare(strict_types=1); }", always(firstStrict)},
	{"strict_types in a function", "function f() { declare(strict_types=1); }", always(firstStrict)},
	{"strict_types after an empty statement", ";declare(strict_types=1);", []compileErrorFrom{{token.PHP74, token.PHP84, firstStrict}}},
	{"strict_types after inline HTML", "?>\n<p>\n<?php declare(strict_types=1);", always(firstStrict)},
	{"strict_types in block mode", "declare(strict_types=1) { }", always("strict_types declaration must not use block mode")},
	{"strict_types with a constant", "declare(STRICT_TYPES=true);", always("declare(STRICT_TYPES) value must be a literal")},
	{"ticks with a negative number", "declare(ticks=-1);", always("declare(ticks) value must be a literal")},
	{"ticks with a magic constant", "declare(ticks=__LINE__);", always("declare(ticks) value must be a literal")},
	{"ticks with an interpolated string", `declare(ticks="$x");`, always("declare(ticks) value must be a literal")},
	{"strict_types with an interpolating heredoc", "declare(strict_types=<<<EOT\n$x\nEOT);", always("declare(strict_types) value must be a literal")},
	{"strict_types with a heredoc", "declare(strict_types=<<<EOT\n1\nEOT);", always("strict_types declaration must have 0 or 1 as its value")},
	{"ticks with a heredoc", "declare(ticks=<<<EOT\n1\nEOT);", nil},
	{"strict_types with 2", "declare(strict_types=2);", always("strict_types declaration must have 0 or 1 as its value")},
	{"strict_types with a string", `declare(strict_types="1");`, always("strict_types declaration must have 0 or 1 as its value")},
	{"declares", "declare(ticks=1);\ndeclare(strict_types=01);\ndeclare(strict_types=0);", nil},
}

func TestCompileErrors(t *testing.T) {
	for _, tc := range compileErrorCases {
		t.Run(tc.name, func(t *testing.T) {
			src := "<?php\n" + tc.src + "\n"
			parsed := false
			for v := token.PHP74; v <= token.Latest; v++ {
				file := parser.ParseFile(src, v)
				if len(file.Errors) > 0 {
					continue
				}
				parsed = true
				var want []string
				for _, w := range tc.want {
					if w.from <= v && v <= w.to {
						want = append(want, w.message)
					}
				}
				var got []string
				for _, f := range (&CompileErrorRule{}).Check(file, src, nil) {
					if f.Code != rule.CompileError || f.Severity != rule.SeverityError {
						t.Errorf("PHP %s: finding %+v has the wrong code or severity", v, f)
					}
					got = append(got, f.Message)
				}
				if !slices.Equal(got, want) {
					t.Errorf("PHP %s: got %q, want %q", v, got, want)
				}
			}
			if !parsed {
				t.Fatal("the source parses on no PHP version")
			}
		})
	}
}

func TestStrictTypesAfterWhatPrecedesTheOpenTag(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"#!/usr/bin/env php\n<?php\ndeclare(strict_types=1);\n", false},
		{"#!php\r\n<?php\ndeclare(strict_types=1);\n", false},
		{"#!php\n<?php declare(strict_types=1); echo 'x';\n", false},
		{"#!php\n\n<?php\ndeclare(strict_types=1);\n", true},
		{" #!php\n<?php\ndeclare(strict_types=1);\n", true},
		{" <?php\ndeclare(strict_types=1);\n", true},
		{"\xef\xbb\xbf<?php\ndeclare(strict_types=1);\n", true},
		{"x<?php declare(strict_types=1);\n", true},
		{"#!php\n<?php ?>x<?php declare(strict_types=1);\n", true},
	} {
		for v := token.PHP74; v <= token.Latest; v++ {
			got := (&CompileErrorRule{}).Check(parser.ParseFile(tc.src, v), tc.src, nil)
			if reported := len(got) == 1 && got[0].Message == firstStrict; reported != tc.want || len(got) > 1 {
				t.Errorf("PHP %s, %q: got %+v, want an error: %v", v, tc.src, got, tc.want)
			}
		}
	}
}

func TestCompileErrorsArePlacedOnTheConstruct(t *testing.T) {
	src := "<?php\nclass A {\n    public function f() {}\n    public function f() {}\n}\n$x = 1 ? 2 : 3 ? 4 : 5;\n"
	got := (&CompileErrorRule{}).Check(parser.ParseFile(src, token.PHP83), src, nil)
	want := []Finding{
		{StartLine: 3, StartCol: 20, EndLine: 3, EndCol: 21},
		{StartLine: 5, StartCol: 5, EndLine: 5, EndCol: 22},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d findings, want %d: %+v", len(got), len(want), got)
	}
	for i, f := range got {
		if f.StartLine != want[i].StartLine || f.StartCol != want[i].StartCol || f.EndLine != want[i].EndLine || f.EndCol != want[i].EndCol {
			t.Errorf("finding %d %q at %d:%d-%d:%d, want %d:%d-%d:%d", i, f.Message, f.StartLine, f.StartCol, f.EndLine, f.EndCol, want[i].StartLine, want[i].StartCol, want[i].EndLine, want[i].EndCol)
		}
	}
}

func TestCompileErrorsWaitForTheFileToParse(t *testing.T) {
	src := "<?php\n$x = 1 ? 2 : 3 ? 4 : 5;\nfunction (\n"
	if got := (&CompileErrorRule{}).Check(parser.ParseFile(src, token.PHP83), src, nil); len(got) != 0 {
		t.Errorf("got %+v for a file with a syntax error", got)
	}
}

func TestCompileErrorRuleReportsCompileError(t *testing.T) {
	if got := (&CompileErrorRule{}).Codes(); !slices.Equal(got, []rule.Code{rule.CompileError}) {
		t.Errorf("codes %v", got)
	}
}
