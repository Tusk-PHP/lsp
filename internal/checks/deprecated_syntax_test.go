package checks

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// deprecatedSyntax returns each finding as "line: covered text | message",
// line 1-based, for a source checked as a PHP 8.6 project.
func deprecatedSyntax(t *testing.T, version, src string) []string {
	t.Helper()
	lines := strings.Split(src, "\n")
	var out []string
	for _, f := range (&DeprecatedSyntaxRule{PHPVersion: version}).Check(parser.ParseFile(src), src, nil) {
		if f.StartLine != f.EndLine {
			t.Fatalf("finding spans lines %d to %d: %+v", f.StartLine, f.EndLine, f)
		}
		text := lines[f.StartLine][f.StartCol:f.EndCol]
		out = append(out, strconv.Itoa(f.StartLine+1)+": "+text+" | "+f.Message)
	}
	return out
}

func expectFindings(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The expectations below were checked against `php -l` and running each case
// on PHP 8.6.0.

func TestDeprecatedSyntaxReservedNames(t *testing.T) {
	src := `<?php
namespace App;
use Foo\Bar as let;
use Foo\Let;
use Foo\{Baz as is, Qux};
use function Foo\helper as is;
use const Foo\VALUE as _;
use Foo\_;
class let {}
interface IS {}
final class _ {}
function let() {}
function READONLY() {}
const is = 1, _ = 2;
define('let', 1);
define("_", 2);
class_alias(Qux::class, 'Is');
`
	expectFindings(t, deprecatedSyntax(t, "8.6", src), []string{
		`3: let | Using "let" as a class name is deprecated since PHP 8.6, it is reserved for a future keyword`,
		`4: Let | Using "let" as a class name is deprecated since PHP 8.6, it is reserved for a future keyword`,
		`5: is | Using "is" as a class name is deprecated since PHP 8.6, it is reserved for a future keyword`,
		`6: is | Using "is" as a function name is deprecated since PHP 8.6, it is reserved for a future keyword`,
		`7: _ | Using "_" as a constant name is deprecated since PHP 8.6`,
		`8: _ | Using "_" as a class name is deprecated since PHP 8.4`,
		`9: let | Using "let" as a class name is deprecated since PHP 8.6, it is reserved for a future keyword`,
		`10: IS | Using "is" as an interface name is deprecated since PHP 8.6, it is reserved for a future keyword`,
		`11: _ | Using "_" as a class name is deprecated since PHP 8.4`,
		`12: let | Using "let" as a function name is deprecated since PHP 8.6, it is reserved for a future keyword`,
		`13: READONLY | A function called "readonly" is deprecated since PHP 8.6, readonly is a keyword`,
		`14: is | Using "is" as a constant name is deprecated since PHP 8.6, it is reserved for a future keyword`,
		`14: _ | Using "_" as a constant name is deprecated since PHP 8.6`,
		`15: 'let' | Using "let" as a constant name is deprecated since PHP 8.6, it is reserved for a future keyword`,
		`16: "_" | Using "_" as a constant name is deprecated since PHP 8.6`,
		`17: 'Is' | Using "is" as a class alias is deprecated since PHP 8.6, it is reserved for a future keyword`,
	})
}

// PHP still allows these: methods, class constants and enum cases called
// `let` or `is`, `_` as a function alias, namespace segments, `let` as a
// variable or property, and a static property or method called `namespace`.
func TestDeprecatedSyntaxAllowedNames(t *testing.T) {
	src := `<?php
namespace Let\Is;
use function Foo\translate as _;
use const Foo\_ as UNDERSCORE;
use Foo\Bar;
class A {
    use SomeTrait;
    const let = 1;
    const is = 2, _ = 3;
    public static $namespace;
    public function let() {}
    public function is(): bool { return true; }
    public static function namespace() {}
    public function run() {
        $let = $this->let() || A::is() || $this->is;
        $f = function () use ($let) { return $let; };
        define('LET_ME', 1);
        $x->define('let', 1, true);
        Foo\define('let', 1, true);
        define(...);
        return _('text');
    }
}
enum E { case let; case is; }
route(class: Handler::class, enum: E::class);
$map = ['x' => function () { const_like(); }];
$o = new class(1) extends Base { const let = 1; function let() {} };
define('namespace', 1);
$s = 'class let {}'; // function let() {}
`
	expectFindings(t, deprecatedSyntax(t, "8.6", src), nil)
}

func TestDeprecatedSyntaxNamespaceClassConstant(t *testing.T) {
	src := `<?php
class A { const NAMESPACE = 'App'; }
interface I { public const string namespace = 'x'; }
enum E { case Namespace; const X = 1, nAmEsPaCe = 2; }
`
	expectFindings(t, deprecatedSyntax(t, "8.6", src), []string{
		`2: NAMESPACE | Declaring a class constant called "namespace" is deprecated since PHP 8.6`,
		`3: namespace | Declaring an interface constant called "namespace" is deprecated since PHP 8.6`,
		`4: Namespace | Declaring an enum constant called "namespace" is deprecated since PHP 8.6`,
		`4: nAmEsPaCe | Declaring an enum constant called "namespace" is deprecated since PHP 8.6`,
	})
}

func TestDeprecatedSyntaxReturnInFinally(t *testing.T) {
	src := `<?php
function f() {
    try {
        return 1;
    } catch (Exception $e) {
        return 2;
    } finally {
        if ($x) {
            return;
        }
        try { return 3; } finally { return 4; }
        $c = function () { return 5; };
        $d = fn () => 6;
        $e = function () { try {} finally { return 7; } };
    }
    return 8;
}
try {} finally { return; }
`
	const msg = " | Returning from a finally block is deprecated since PHP 8.6, it discards the exception or the value returned by the try block"
	expectFindings(t, deprecatedSyntax(t, "8.6", src), []string{
		"9: return" + msg,
		"11: return" + msg,
		"11: return" + msg,
		"14: return" + msg,
		"18: return" + msg,
	})
}

func TestDeprecatedSyntaxConstructorAndDestructor(t *testing.T) {
	src := `<?php
class A {
    public function __construct() {
        if ($x) { return; }
        $f = function () { return 1; };
        $g = fn () => yield 1;
        $h = new class { public function run() { return 2; } };
        $this->return = $return ?? self::RETURN;
        return $this->build([1, 2]);
    }
    public function __DESTRUCT() { return null; }
    public function build() { return 3; }
}
class B { function __construct() { yield 1; yield 2; } }
trait T { function __destruct() { yield; } }
$o = new class { function __construct() { return 4; } };
function __construct() { return 5; }
`
	expectFindings(t, deprecatedSyntax(t, "8.6", src), []string{
		"9: return $this->build([1, 2]) | Returning a value from a constructor is deprecated since PHP 8.6",
		"11: return null | Returning a value from a destructor is deprecated since PHP 8.6",
		"14: yield | Making a constructor a generator with yield is deprecated since PHP 8.6",
		"15: yield | Making a destructor a generator with yield is deprecated since PHP 8.6",
		"16: return 4 | Returning a value from a constructor is deprecated since PHP 8.6",
	})
}

func TestDeprecatedSyntaxDebugInfoReturnType(t *testing.T) {
	src := `<?php
class A { function __debugInfo(): ?array { return []; } }
class B { function __debugInfo(): array|null { return []; } }
interface I { function __debugInfo(): null|ARRAY; }
class C { function __debugInfo(): array { return []; } }
class D { function __debugInfo() { return []; } }
function __debugInfo(): ?array { return []; }
`
	const msg = " | A nullable return type for __debugInfo() is deprecated since PHP 8.6, declare array and return an empty array instead of null"
	expectFindings(t, deprecatedSyntax(t, "8.6", src), []string{
		"2: ?array" + msg,
		"3: array|null" + msg,
		"4: null|ARRAY" + msg,
	})
}

func TestDeprecatedSyntaxArguments(t *testing.T) {
	src := `<?php
namespace App;
define('A', 1, false);
\define('B', 2, true);
sort($items, SORT_LOCALE_STRING);
$flags = \SORT_LOCALE_STRING | SORT_FLAG_CASE;
define('C', 3);
$x = Foo::SORT_LOCALE_STRING;
`
	expectFindings(t, deprecatedSyntax(t, "8.6", src), []string{
		"3: false | define()'s third argument is deprecated since PHP 8.6: case-insensitive constants are no longer supported, so it is ignored; remove it",
		"4: true | define()'s third argument is deprecated since PHP 8.6: case-insensitive constants are no longer supported, so it is ignored; remove it",
		"5: SORT_LOCALE_STRING | SORT_LOCALE_STRING is deprecated since PHP 8.6, use one of Collator's sort methods instead",
		"6: SORT_LOCALE_STRING | SORT_LOCALE_STRING is deprecated since PHP 8.6, use one of Collator's sort methods instead",
	})
}

// A project that still supports an older PHP gets only the deprecations of
// the versions it no longer supports.
func TestDeprecatedSyntaxFollowsLowestPHPVersion(t *testing.T) {
	src := `<?php
class _ {}
class let {}
`
	expectFindings(t, deprecatedSyntax(t, "8.3", src), nil)
	expectFindings(t, deprecatedSyntax(t, "8.5", src), []string{
		`2: _ | Using "_" as a class name is deprecated since PHP 8.4`,
	})
	if got := deprecatedSyntax(t, "", src); len(got) != 2 {
		t.Fatalf("no version should report every deprecation, got %v", got)
	}
}
