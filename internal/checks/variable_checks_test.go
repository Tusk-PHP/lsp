package checks

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
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
