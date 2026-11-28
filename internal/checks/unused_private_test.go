package checks

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestUnusedPrivateRule(t *testing.T) {
	checker := &UnusedPrivateRule{}

	t.Run("unused private method detected", func(t *testing.T) {
		source := `<?php
class Foo {
    private function unusedMethod(): void {}
    public function bar(): void {
        echo "hello";
    }
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unused-private-method"})
		if findings[0].Message != "Private method 'Foo::unusedMethod' is never used" {
			t.Errorf("unexpected message: %s", findings[0].Message)
		}
	})

	t.Run("used private method via this not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private function helper(): string { return "ok"; }
    public function bar(): void {
        echo $this->helper();
    }
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("used private method via self not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private static function helper(): string { return "ok"; }
    public function bar(): void {
        echo self::helper();
    }
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("unused private property detected", func(t *testing.T) {
		source := `<?php
class Foo {
    private string $unused = "hello";
    public function bar(): void {
        echo "world";
    }
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unused-private-property"})
	})

	t.Run("used private property via this not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private string $name = "hello";
    public function bar(): string {
        return $this->name;
    }
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("magic methods never flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private function __construct() {}
    private function __toString(): string { return ""; }
    private function __clone(): void {}
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("private method used as callable string not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private function handler(): void {}
    public function register(): void {
        $cb = [$this, 'handler'];
    }
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("protected and public methods never flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    protected function protectedUnused(): void {}
    public function publicUnused(): void {}
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("severity is info for methods hint for properties", func(t *testing.T) {
		source := `<?php
class Foo {
    private string $unused = "";
    private function unusedMethod(): void {}
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		if len(findings) != 2 {
			t.Fatalf("expected 2 findings, got %d", len(findings))
		}
		// Methods checked first, then properties
		methF := findings[0]
		propF := findings[1]
		if propF.Code != "unused-private-property" || propF.Severity != rule.SeverityHint {
			t.Errorf("property finding: code=%s sev=%d", propF.Code, propF.Severity)
		}
		if methF.Code != "unused-private-method" || methF.Severity != rule.SeverityInfo {
			t.Errorf("method finding: code=%s sev=%d", methF.Code, methF.Severity)
		}
	})

	t.Run("used private static property not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private static int $count = 0;
    public function increment(): void {
        self::$count++;
    }
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})
}

func TestUnusedPrivateSeesUsesAfterBracesInStrings(t *testing.T) {
	// WordPress's WP_Block_Processor compares characters with '{' and '}' in
	// strings; counting those braces ended the class early, so later reads of
	// a private property or calls of a private method were missed.
	src := `<?php
class WP_Block_Processor {
    private $last_json_error = 0;
    private function parse_json() {}
    public function scan( $text ) {
        $has_json = '{' === $text[0];
        $closes   = "}}" === substr( $text, -2 );
    }
    public function get_last_json_error(): int {
        $this->parse_json();
        return $this->last_json_error;
    }
}
`
	if got := (&UnusedPrivateRule{}).Check(parser.ParseFile(src), src, nil); len(got) != 0 {
		t.Errorf("used private members reported unused: %+v", got)
	}
}

func unusedPrivateMessages(src string, idx *symbols.Index) []string {
	var got []string
	for _, f := range (&UnusedPrivateRule{}).Check(parser.ParseFile(src), src, idx) {
		got = append(got, f.Message)
	}
	return got
}

func TestUnusedPrivateReadsOnlyCode(t *testing.T) {
	src := `<?php
class Foo {
    private function inComment(): void {}
    private function helper(): void {}
    private function inString(): void {}
    private function inHtml(): void {}
    private string $note = '';
    private static int $total = 0;
    public function run(): void {
        // $this->inComment();
        /* self::inComment() */
        # [$this, 'inComment']
        $this->helperX();
        self::$totalX = 1;
        echo '->inString', "call ::inString later", "$this->note->x";
        ?><p>$this->inHtml() self::inHtml() 'inHtml'</p><?php
    }
    public function helperX(): void {}
    public static int $totalX = 0;
}
`
	want := []string{
		"Private method 'Foo::inComment' is never used",
		"Private method 'Foo::helper' is never used",
		"Private method 'Foo::inString' is never used",
		"Private method 'Foo::inHtml' is never used",
		"Private property 'Foo::$total' is never used",
	}
	if got := unusedPrivateMessages(src, nil); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnusedPrivateCountsEveryFormOfUse(t *testing.T) {
	src := `<?php
class Foo {
    private function nullsafe(): void {}
    private function caseless(): void {}
    private function interpolated(): string { return ''; }
    private function staticCallable(): void {}
    private function arrayCallable(): void {}
    private function keyword(): void {}
    private string $checked = '';
    private string $inHeredoc = '';
    private ?Foo $next = null;
    public function run(): string {
        $this->next?->nullsafe();
        $this->CASELESS();
        $cb = array($this, 'arrayCallable');
        $other = 'Foo::staticCallable';
        $this->new;
        $exists = property_exists($this, 'checked');
        return "{$this->interpolated()}" . <<<EOT
            $this->inHeredoc
            EOT;
    }
    private function new(): void { $this->keyword(); }
}
`
	if got := unusedPrivateMessages(src, nil); len(got) != 0 {
		t.Errorf("used private members reported unused: %q", got)
	}
}

func TestUnusedPrivateStaysQuietOnDynamicAccess(t *testing.T) {
	for name, body := range map[string]string{
		"dynamic method":             "$this->$name();",
		"braced dynamic method":      "$this->{$name}();",
		"nullsafe dynamic method":    "$this?->$name();",
		"static variable call":       "static::$name();",
		"self variable call":         "self::$name();",
		"own class variable call":    "Foo::$name();",
		"$this static variable call": "$this::$name();",
		"braced static access":       "static::{$name}();",
		"variable callable":          "call_user_func([$this, $name]);",
		"array() callable":           "call_user_func(array($this, $name));",
		"class-name callable":        "call_user_func([self::class, $name]);",
		"own class-name callable":    "call_user_func([Foo::class, $name]);",
		"qualified own class-name":   "call_user_func([\\Foo::class, $name]);",
		"__CLASS__ callable":         "call_user_func([__CLASS__, $name]);",
		"computed callable name":     "call_user_func([static::class, 'help' . $name]);",
		"interpolated callable name": `call_user_func([$this, "help$name"]);`,
		"braced interpolated name":   `$this->{"help{$name}"}();`,
		"computed callable":          "call_user_func([$this, $this->pick()]);",
		"constant callable name":     "call_user_func([$this, M]);",
		"included file":              "include __DIR__ . '/view.php';",
		"required file":              "require_once 'view.php';",
	} {
		src := "<?php\nclass Foo {\n    private function helper(): void {}\n    private string $value = '';\n    public function run(string $name): void { " + body + " }\n}\n"
		for _, msg := range unusedPrivateMessages(src, nil) {
			if msg == "Private method 'Foo::helper' is never used" {
				t.Errorf("%s: %s", name, msg)
			}
		}
	}
}

// Only a dynamic call that can reach the class's own methods says nothing
// about which of them are used: a dynamic property read, a call on another
// object or class, or a name with a different literal prefix doesn't.
func TestUnusedPrivateDynamicAccessThatCantReachTheMethod(t *testing.T) {
	for name, body := range map[string]string{
		"dynamic property read":               "return $this->$name;",
		"variable variable":                   "return $this->$$name;",
		"braced property read on an other":    "return $theme->{'Author URI'};",
		"call on another object":              "$other->$name();",
		"call on a property":                  "$this->items->$name();",
		"call on another class":               "Other::$name();",
		"static call on another object":       "$other::$name();",
		"parent call":                         "parent::$name();",
		"callable of another object":          "call_user_func([$other, $name]);",
		"callable of another class":           "call_user_func([Other::class, $name]);",
		"another literal prefix":              "call_user_func([$this, 'format_' . $name]);",
		"another interpolated prefix":         `$this->{"format_$name"}();`,
		"a method named include":              "$this->include();",
		"dangling access in an interpolation": `echo "{$this->}";`,
	} {
		src := "<?php\nclass Foo {\n    private function helper(): void {}\n    public function run(string $name, $other, $theme) { " + body + " }\n}\n"
		if got := unusedPrivateMessages(src, nil); !slices.Equal(got, []string{"Private method 'Foo::helper' is never used"}) {
			t.Errorf("%s: got %q", name, got)
		}
	}
}

// A variable used as a method name has the literal prefixes of the values
// assigned to it; one it may get otherwise could name any method.
func TestUnusedPrivateMethodNameVariables(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want       []string
	}{
		{"one literal prefix", "$m = 'format_' . $key; return self::$m();", []string{"Private method 'Foo::helper' is never used"}},
		{"a matching prefix", "$m = 'help' . $key; return $this->$m();", nil},
		{"one of several prefixes matches", "$m = 'format_' . $key; $m = 'helper'; return $this->$m();", nil},
		{"a whole literal", "$m = 'format_x'; return call_user_func([$this, $m]);", []string{"Private method 'Foo::helper' is never used"}},
		{"a computed value", "$m = strtolower($key); return $this->$m();", nil},
		{"a parameter", "return $this->$key();", nil},
		{"a foreach target", "$m = 'format_'; foreach ($keys as $m) { $this->$m(); }", nil},
		{"a destructuring", "$m = 'format_'; [$m] = $keys; return $this->$m();", nil},
		{"a reference", "$m = 'format_'; fill($keys, &$m); return $this->$m();", nil},
		{"never assigned", "return $this->$m();", nil},
		{"a list()", "$m = 'format_'; list($m) = $keys; return $this->$m();", nil},
		{"a closure parameter", "$m = 'format_'; return array_map(function ($m) { return $this->$m(); }, $keys);", nil},
		{"a destructuring after a subscript", "$m = 'format_'; [$keys[0], $m] = $keys; return $this->$m();", nil},
		{"a literal that starts an expression", "$m = 'format_x' ?: $key; return $this->$m();", nil},
		{"an interpolated string that starts an expression", `$m = "format_{$key}" . $key; return $this->$m();`, []string{"Private method 'Foo::helper' is never used"}},
		{"an interpolated string that starts with a variable", `$m = "$key"; return $this->$m();`, nil},
		{"an interpolated string with an escape before its variable", `$m = "format\t$key"; return $this->$m();`, nil},
		{"an array left open", "$m = 'format_'; $a = [$m; return $this->$m();", []string{"Private method 'Foo::helper' is never used"}},
	} {
		src := "<?php\nclass Foo {\n    private function helper(): void {}\n    private function format_x(): void {}\n    public function run(string $key, array $keys) { " + c.body + " }\n}\n"
		got := unusedPrivateMessages(src, nil)
		got = slices.DeleteFunc(got, func(m string) bool { return m == "Private method 'Foo::format_x' is never used" })
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestUnusedPrivateStillReportsBesideArraysThatAreNotDynamicCallables(t *testing.T) {
	src := `<?php
class Foo {
    private function helper(): void {}
    private function never(): void {}
    private array $items = [];
    public function run(): void {
        call_user_func([$this, 'helper'], 1);
        [$a, $b] = [1, 2];
        $x = [self::FIRST, $a];
        $y = [$this->items, $a];
        $z = [Foo::class];
        $w = [1, $a];
    }
}
`
	if got := unusedPrivateMessages(src, nil); !slices.Equal(got, []string{"Private method 'Foo::never' is never used"}) {
		t.Errorf("got %q", got)
	}
}

func TestUnusedPrivateReadsTraitsAsCode(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Mentions.php", `<?php
trait Mentions {
    // $this->helper(), self::$count, self::LIMIT
    public function describe(): string { return 'helper count LIMIT'; }
    public function run(): void { $this->used(); }
}`)
	src := `<?php
class Foo {
    use Mentions;
    private const LIMIT = 1;
    private static int $count = 0;
    private function helper(): void {}
    private function used(): void {}
}`
	idx.IndexFile("file:///Foo.php", src)
	want := []string{
		"Private method 'Foo::helper' is never used",
		"Private property 'Foo::$count' is never used",
		"Private constant 'Foo::LIMIT' is never used",
	}
	if got := unusedPrivateMessages(src, idx); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnusedPrivatePropertiesCountOnlyPropertyExistsLiterals(t *testing.T) {
	src := `<?php
class Foo {
    private $scope;
    private $checked;
    private $nested;
    public function run(array $a): array {
        $exists = property_exists($this, 'checked') && property_exists(get($a, [1, 2]), 'nested');
        $noCall = property_exists;
        $noName = property_exists($this);
        return ['scope' => $a];
    }
}
`
	if got := unusedPrivateMessages(src, nil); !slices.Equal(got, []string{"Private property 'Foo::$scope' is never used"}) {
		t.Errorf("got %q", got)
	}
	for name, call := range map[string]string{
		"variable name":     "property_exists($this, $name)",
		"computed name":     "property_exists($this, 'sc' . $name)",
		"interpolated name": `property_exists($this, "sc$name")`,
	} {
		src := "<?php\nclass Foo {\n    private $scope;\n    public function run(string $name): bool { return " + call + "; }\n}\n"
		if got := unusedPrivateMessages(src, nil); len(got) != 0 {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestUnusedPrivateCountsReflectionOfItsOwnClass(t *testing.T) {
	const (
		method   = "Private method 'Foo::helper' is never used"
		property = "Private property 'Foo::$value' is never used"
		constant = "Private constant 'Foo::LIMIT' is never used"
	)
	for name, tc := range map[string]struct {
		body string
		want []string
	}{
		"get_object_vars($this)":                 {"return get_object_vars( $this );", []string{method, constant}},
		"get_class_vars(self::class)":            {"return \\get_class_vars(self::class);", []string{method, constant}},
		"(array) $this":                          {"return (array) $this;", []string{method, constant}},
		"(array) $this before a ternary":         {"return (array) $this ?: [];", []string{method, constant}},
		"foreach ($this as ...)":                 {"foreach ($this as $k => $v) {} return [];", []string{method, constant}},
		"get_class_methods($this)":               {"return get_class_methods($this);", []string{property, constant}},
		"get_class_methods(self::class)":         {"return get_class_methods(SELF::class);", []string{property, constant}},
		"get_class_methods(static::class)":       {"return get_class_methods(static::class);", []string{property, constant}},
		"get_class_methods(__CLASS__)":           {"return get_class_methods(__CLASS__);", []string{property, constant}},
		"new ReflectionClass($this)":             {"return (new ReflectionClass($this))->getMethods();", nil},
		"new \\ReflectionClass(self::class)":     {"$r = new \\ReflectionClass(self::class); return [];", nil},
		"new ReflectionObject(static::class)":    {"$r = new ReflectionObject(static::class); return [];", nil},
		"get_object_vars of another object":      {"return get_object_vars($other);", []string{method, property, constant}},
		"get_object_vars of a property":          {"return get_object_vars($this->items);", []string{method, property, constant}},
		"get_object_vars without a call":         {"return [get_object_vars];", []string{method, property, constant}},
		"(array) of a property":                  {"return (array) $this->items;", []string{method, property, constant}},
		"(array) of a nullsafe property":         {"return (array) $this?->items;", []string{method, property, constant}},
		"(array) of an offset":                   {"return (array) $this[0];", []string{method, property, constant}},
		"(array) of another variable":            {"return (array) $other;", []string{method, property, constant}},
		"array() of $this":                       {"return array($this);", []string{method, property, constant}},
		"(array) without parentheses around it":  {"return [array, $this];", []string{method, property, constant}},
		"(array) not closed":                     {"return (array $this);", []string{method, property, constant}},
		"foreach over another variable":          {"foreach ($other as $v) {} return [];", []string{method, property, constant}},
		"foreach over a property":                {"foreach ($this->items as $v) {} return [];", []string{method, property, constant}},
		"foreach without parentheses":            {"return [foreach];", []string{method, property, constant}},
		"get_class_methods of another class":     {"return get_class_methods(Other::class);", []string{method, property, constant}},
		"get_class_methods of a qualified class": {"return get_class_methods(\\Other::class);", []string{method, property, constant}},
		"get_class_methods with a second value":  {"return get_class_methods($this, 1);", []string{method, property, constant}},
		"ReflectionClass of another class":       {"$r = new ReflectionClass(Other::class); return [];", []string{method, property, constant}},
		"ReflectionClass without new":            {"$r = ReflectionClass($this); return [];", []string{method, property, constant}},
	} {
		src := "<?php\nclass Foo {\n    private const LIMIT = 1;\n    private string $value = '';\n    private function helper(): void {}\n    public function run($other): array { " + tc.body + " }\n}\n"
		if got := unusedPrivateMessages(src, nil); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

func TestUnusedPrivateReadsOnlyTheClassItsMemberBelongsTo(t *testing.T) {
	src := `<?php
class One { private $x = '{'; private function y() {} }
class Two { private $x; private function y() {} public function f() { return $this->x . $this->y(); } }
`
	want := []string{
		"Private method 'One::y' is never used",
		"Private property 'One::$x' is never used",
	}
	if got := unusedPrivateMessages(src, nil); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnusedPrivateMethodNameVariableAtTheStartOfAnAnonymousClassLine(t *testing.T) {
	src := "<?php\n$m = 'format_'; $o = new class { private function helper(): void {} private function format_x(): void {} public function run() { $m = 'help'; return $this->$m(); } };\n"
	if got := unusedPrivateMessages(src, nil); len(got) != 0 {
		t.Errorf("both prefixes assigned to $m, the first at the start of the class's line, reach the methods: got %q", got)
	}
}

func TestUnusedPrivateReadsAnUnterminatedBracedAccessAsNoCall(t *testing.T) {
	src := "<?php\nclass Foo {\n    private function helper(): void {}\n    public function run(string $key) { return $this->{$key"
	if got := unusedPrivateMessages(src, nil); !slices.Equal(got, []string{"Private method 'Foo::helper' is never used"}) {
		t.Errorf("got %q", got)
	}
}
