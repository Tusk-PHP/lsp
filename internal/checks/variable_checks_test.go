package checks

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// ---- UndefinedVariableRule tests ----

func TestUndefinedVariableRule(t *testing.T) {
	rule := &UndefinedVariableRule{}

	t.Run("simple defined variable is not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    $x = 1;
    echo $x;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("undefined read is flagged", func(t *testing.T) {
		source := `<?php
function run() {
    echo $undefined;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"undefined-variable"})
	})

	t.Run("parameter is not flagged", func(t *testing.T) {
		source := `<?php
function run($param) {
    echo $param;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("superglobals are not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    $a = $_GET['id'];
    $b = $_POST['name'];
    $c = $_SERVER['HTTP_HOST'];
    $d = $_SESSION['user'];
    $e = $_COOKIE['token'];
    $f = $_FILES['upload'];
    $g = $_ENV['APP_KEY'];
    $h = $_REQUEST['data'];
    $i = $GLOBALS['foo'];
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("$this is not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private $bar = 1;
    public function run() {
        return $this->bar;
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("foreach value binding is not flagged", func(t *testing.T) {
		source := `<?php
function run(array $items) {
    foreach ($items as $item) {
        echo $item;
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("foreach key binding is not flagged", func(t *testing.T) {
		source := `<?php
function run(array $items) {
    foreach ($items as $key => $value) {
        echo $key . ': ' . $value;
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("catch binding is not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    try {
        doSomething();
    } catch (\Exception $e) {
        echo $e->getMessage();
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("closure use binding is not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    $value = 42;
    $fn = function() use ($value) {
        return $value;
    };
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("destructuring assignment is not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    [$a, $b] = [1, 2];
    echo $a + $b;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("extract() call skips scope", func(t *testing.T) {
		source := `<?php
function run(array $data) {
    extract($data);
    echo $foo; // $foo might come from extract
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("include call skips scope", func(t *testing.T) {
		source := `<?php
function run() {
    include 'config.php';
    echo $configVar; // might come from include
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("require call skips scope", func(t *testing.T) {
		source := `<?php
function run() {
    require 'bootstrap.php';
    echo $appVar;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("nil file returns nil", func(t *testing.T) {
		findings := rule.Check(nil, "", nil)
		assertNoFindings(t, findings)
	})
}

// ---- UnusedVariableRule tests ----

func TestUnusedVariableRule(t *testing.T) {
	rule := &UnusedVariableRule{}

	t.Run("used variable not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    $x = 1;
    echo $x;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("unused assignment flagged", func(t *testing.T) {
		source := `<?php
function run() {
    $x = computeSomething();
    return 0;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unused-variable"})
		if findings[0].Severity != SeverityHint {
			t.Errorf("expected SeverityHint, got %d", findings[0].Severity)
		}
	})

	t.Run("parameter not flagged even if not read", func(t *testing.T) {
		source := `<?php
function run($unused) {
    return 0;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("variable named $_ not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    $_ = someCall();
    return 0;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("variable named $_foo not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    $_unused = someCall();
    return 0;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("foreach value binding not flagged", func(t *testing.T) {
		source := `<?php
function run(array $items) {
    foreach ($items as $item) {
        // not used — but this is a foreach binding, not a plain variable
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("catch binding not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    try {
        doSomething();
    } catch (\Exception $e) {
        // $e not used — but it's a catch binding
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("closure-use origin not flagged as unused", func(t *testing.T) {
		source := `<?php
function run() {
    $value = 42;
    $fn = function() use ($value) {
        return $value * 2;
    };
    return $fn();
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		// $value is captured by the closure (origin of a BindingClosureUse), not unused
		// $fn is used (called), not unused
		assertNoFindings(t, findings)
	})

	t.Run("closure-use capture variable not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    $value = 42;
    $fn = function() use ($value) {
        // $value is captured but not read inside closure — it's a closure-use binding
        // The outer $value is still "used" because it's the origin of a closure capture
    };
    return $fn();
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		// $value outer is the origin of a closure-use binding, so not flagged as unused
		assertNoFindings(t, findings)
	})

	t.Run("extract() call skips scope", func(t *testing.T) {
		source := `<?php
function run(array $data) {
    extract($data);
    $local = 1;
    return 0;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("unused has unnecessary tag", func(t *testing.T) {
		source := `<?php
function run() {
    $x = 1;
    return 0;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		if len(findings) != 1 {
			t.Fatalf("expected 1 finding, got %d", len(findings))
		}
		hasTag := false
		for _, tag := range findings[0].Tags {
			if tag == TagUnnecessary {
				hasTag = true
			}
		}
		if !hasTag {
			t.Error("expected TagUnnecessary")
		}
	})

	t.Run("nil file returns nil", func(t *testing.T) {
		findings := rule.Check(nil, "", nil)
		assertNoFindings(t, findings)
	})

	t.Run("reference assignment not flagged", func(t *testing.T) {
		source := `<?php
function run() {
    $original = 1;
    $ref = &$original;
    return $original;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		// $ref is a reference binding — should not be flagged
		// $original is used (by reference and returned)
		assertNoFindings(t, findings)
	})
}

func TestUndefinedVariableComparisonsAndReassignment(t *testing.T) {
	source := `<?php
function f($p) {
    $x = 1;
    $x = 2;
    if ($x === 1 || $x == 2 || $x !== 3 || $x != 4 || $x <= 5 || $x >= 6 || ($x <=> 7)) {
        return $p === 1;
    }
    foo($y = 1);
    foo($y = 2);
    return $y;
}
`
	if findings := checkUndefinedVariables(source, nil); len(findings) != 0 {
		t.Errorf("unexpected findings: %+v", findings)
	}

	source = "<?php\nfunction f() {\n    return $missing === 1;\n}\n"
	findings := checkUndefinedVariables(source, nil)
	if len(findings) != 1 || findings[0].Message != "Undefined variable $missing" {
		t.Errorf("want $missing reported, got %+v", findings)
	}
}

func TestUnusedVariableCountsComparisonAsRead(t *testing.T) {
	source := "<?php\nfunction f() {\n    $x = 1;\n    return $x === 1;\n}\n"
	if findings := checkUnusedVariables(source); len(findings) != 0 {
		t.Errorf("unexpected findings: %+v", findings)
	}
}

func TestUndefinedVariableNotFlagged(t *testing.T) {
	cases := map[string]string{
		"array key on write":        "function f($a) { $k = 1; $a[$k] = 2; $this->h[$k] = 3; return $a; }",
		"foreach destructure twice": "function f($x) { foreach ($x as [$a, $b]) { echo $a; } foreach ($x as [$a, $b]) { echo $b; } }",
		"foreach key twice":         "function f($x) { foreach ($x as $k => $v) { echo $k; } foreach ($x as $k => $v) { echo $v; } }",
		"new array by subscript":    "function f() { $a[] = 1; $b['x']['y'] = 2; return [$a, $b]; }",
		"global and static":         "function f() { global $a, $b; static $c; static $d = 1; return [$a, $b, $c, $d]; }",
		"property declarations":     "class C { public $a; protected ?string $b = null, $c; public static $d; function f() { return 1; } }",
		"anonymous class property":  "function f() { return new class(1) { public $a; private int $b; }; }",
		"static property":           "class C { function f() { self::$a = 1; return static::$b; } }",
		"out argument":              "function g(&$o) {} function f() { g($o); return $o; }",
		"out argument of a method":  "function f($q) { $q->fetch($o); Foo::fetch($p); new Foo($r); return [$o, $p, $r]; }",
		"out variadic argument":     "function g($a, &...$o) {} function f() { g(1, $o, $p); return [$o, $p]; }",
		"out argument of unknown":   "function f() { unknown_function($o); return $o; }",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			source := "<?php\n" + body + "\n"
			findings := (&UndefinedVariableRule{}).Check(parser.ParseFile(source), source, nil)
			if len(findings) != 0 {
				t.Errorf("unexpected findings: %+v", findings)
			}
		})
	}
}

func TestUndefinedVariableStillFlagged(t *testing.T) {
	cases := map[string]string{
		"argument of a by-value function": "function g($a) {} function f() { g($missing); }",
		"condition":                       "function f() { if ($missing) { return 1; } }",
		"isset-like keyword":              "function f() { echo($missing); }",
		"array element":                   "function g(&$a) {} function f() { g([$missing]); }",
		"nested expression":               "function g(&$a) {} function f() { g($missing + 1); }",
		"dynamic static method":           "class C { function f() { return self::$missing(); } }",
		"subscript read":                  "function f() { return $missing[0]; }",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			source := "<?php\n" + body + "\n"
			findings := (&UndefinedVariableRule{}).Check(parser.ParseFile(source), source, nil)
			if len(findings) != 1 || findings[0].Message != "Undefined variable $missing" {
				t.Errorf("want $missing reported once, got %+v", findings)
			}
		})
	}
}

func TestUnusedVariableNotFlagged(t *testing.T) {
	cases := map[string]string{
		"parameter default":           "function f($a = null, array $b = []) { return 1; }",
		"closure parameter default":   "function f() { return function ($a = null) { return 1; }; }",
		"promoted parameter":          "class C { function __construct(public string $a = 'x') {} }",
		"property default":            "class C { public $a = 1; protected string $b = 'x'; }",
		"static property write":       "class C { function f($m) { static::$a = $m; } }",
		"read in a string":            "function f() { $a = 1; $b = 2; $c = 3; return \"$a {$b} \\\\$c\"; }",
		"read in a multi-line string": "function f() { $a = 1; return \"x\n  $a\"; }",
		"array key read":              "function f($x) { $k = 1; $x[$k] = 2; return $x; }",
		"captured after an arrow fn":  "function f($x) { $a = 1; $b = array_map(fn ($v) => $v, $x); return function () use ($a, $b) { return 1; }; }",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			source := "<?php\n" + body + "\n"
			if findings := checkUnusedVariables(source); len(findings) != 0 {
				t.Errorf("unexpected findings: %+v", findings)
			}
		})
	}
}

func TestUnusedVariableEscapedInStringStillFlagged(t *testing.T) {
	source := "<?php\nfunction f() { $a = 1; return \"\\$a\"; }\n"
	if findings := checkUnusedVariables(source); len(findings) != 1 {
		t.Errorf("want $a reported, got %+v", findings)
	}
}

func TestPropertiesOfClassWithSeveralInterfaces(t *testing.T) {
	source := "<?php\nclass C extends B implements D, E\n{\n    public $a;\n    protected ?string $b = null;\n}\n"
	if findings := (&UndefinedVariableRule{}).Check(parser.ParseFile(source), source, nil); len(findings) != 0 {
		t.Errorf("unexpected undefined findings: %+v", findings)
	}
	if findings := checkUnusedVariables(source); len(findings) != 0 {
		t.Errorf("unexpected unused findings: %+v", findings)
	}
}

func TestUndefinedVariableBuiltinOutArguments(t *testing.T) {
	index := symbols.NewIndex()
	index.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.3", Extensions: []string{"pcre", "standard"}})

	for _, call := range []string{"preg_match($p, $s, $o)", "preg_match_all($p, $s, $o)", "str_replace($p, $s, $s, $o)", "exec($p, $o)", "\\preg_match($p, $s, $o)"} {
		source := "<?php\nfunction f($p, $s) { " + call + "; return $o; }\n"
		if findings := (&UndefinedVariableRule{}).Check(parser.ParseFile(source), source, index); len(findings) != 0 {
			t.Errorf("%s: unexpected findings: %+v", call, findings)
		}
	}

	for _, call := range []string{"count($o)", "preg_match($o, $s)", "str_replace($o, $s, $s)"} {
		source := "<?php\nfunction f($s) { return " + call + "; }\n"
		if findings := (&UndefinedVariableRule{}).Check(parser.ParseFile(source), source, index); len(findings) != 1 {
			t.Errorf("%s: want $o reported, got %+v", call, findings)
		}
	}
}
