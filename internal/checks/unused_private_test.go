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
		"dynamic method":                      "$this->$name();",
		"braced dynamic method":               "$this->{$name}();",
		"variable variable":                   "$this->$$name;",
		"static variable call":                "static::$name();",
		"braced static access":                "static::{$name}();",
		"variable callable":                   "call_user_func([$this, $name]);",
		"array() callable":                    "call_user_func(array($this, $name));",
		"class-name callable":                 "call_user_func([self::class, $name]);",
		"__CLASS__ callable":                  "call_user_func([__CLASS__, $name]);",
		"computed callable name":              "call_user_func([static::class, 'run' . $name]);",
		"interpolated callable name":          `call_user_func([$this, "run$name"]);`,
		"dangling access in an interpolation": `echo "{$this->}";`,
		"constant callable name":              "call_user_func([$this, M]);",
	} {
		src := "<?php\nclass Foo {\n    private function helper(): void {}\n    private string $value = '';\n    public function run(string $name): void { " + body + " }\n}\n"
		for _, msg := range unusedPrivateMessages(src, nil) {
			if msg == "Private method 'Foo::helper' is never used" {
				t.Errorf("%s: %s", name, msg)
			}
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
