package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestClosureParametersFromTheCallTheyArePassedTo(t *testing.T) {
	src := `<?php
namespace App;
class User {}
class Repo {
    /** @param callable(User): mixed $f */
    public static function each(callable $f): void {}
    /** @param callable(User): mixed $f */
    public function map(string $label, callable $f): void {}
}
class Box {
    /** @param callable(User): mixed $f */
    public function __construct(callable $f) {}
}
/** @param callable(User): mixed $f */
function each_user(callable $f): void {}
/**
 * @template T
 * @param T $value
 * @param callable(T): mixed $f
 */
function tap_with($value, callable $f): void {}
function run(Repo $repo, $fn, $cls): void {
    Repo::each(function ($a) {
        echo $a; // static call
    });
    new Box(function ($b) {
        echo $b; // constructor
    });
    each_user(function ($c) {
        echo $c; // function
    });
    $repo->map(f: function ($d) {
        echo $d; // named argument
    }, label: 'x');
    tap_with(new User(), function ($e) {
        echo $e; // template from an earlier argument
    });
    $fn(function ($f) {
        echo $f; // dynamic callee
    });
    new $cls(function ($g) {
        echo $g; // dynamic class
    });
    $list = [function ($h) {
        echo $h; // in an array
    }];
    echo $repo?->map('x', function ($i) {
        echo $i; // nullsafe call
    });
    exit(function ($j) {
        echo $j; // exit argument
    });
    function ($k) {
        echo $k; // closure statement
    };
    #[Pure] static fn ($l) => $l; // attributed arrow function
    Repo::each(#[Pure] function ($m) {
        echo $m; // attributed closure
    });
    foreach ($fn() as $item) {
        echo $item; // foreach over a dynamic call
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///c.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
	lines := SplitLines(src)
	for _, c := range []struct{ v, marker, want string }{
		{"$a", "static call", `App\User`},
		{"$b", "constructor", `App\User`},
		{"$c", "function", `App\User`},
		{"$d", "named argument", `App\User`},
		{"$e", "template from an earlier argument", `App\User`},
		{"$f", "dynamic callee", ``},
		{"$g", "dynamic class", ``},
		{"$h", "in an array", ``},
		{"$i", "nullsafe call", `App\User`},
		{"$item", "foreach over a dynamic call", ``},
		{"$m", "attributed closure", `App\User`},
		{"$j", "exit argument", ``},
		{"$k", "closure statement", ``},
		{"$l", "attributed arrow function", ``},
	} {
		line := markedLine(t, lines, c.marker)
		pos := textpos.Position{Line: line, Character: strings.Index(lines[line], c.v) + 1}
		if got := r.ResolveVariableTypeTyped(c.v, lines, pos, file).FullString(); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}

func TestClosureReturnTypes(t *testing.T) {
	r, ctx := expressionFormsFixture(t)
	for expr, want := range map[string]string{
		"fn () => new User()":                        `Closure(): App\User`,
		"function () { $x = 1; return new User(); }": `Closure(): App\User`,
		"function () { return; }":                    `Closure()`,
		"function (): Admin { return new User(); }":  `Closure(): App\Admin`,
	} {
		if got := r.ExpressionType(expr, ctx).FullString(); got != want {
			t.Errorf("ExpressionType(%q) = %q, want %q", expr, got, want)
		}
	}
}

func TestClosureOutsideAnyCall(t *testing.T) {
	src := "<?php\n(function ($u) {\n    echo $u;\n});\n"
	lines := SplitLines(src)
	if got := NewResolver(symbols.NewIndex()).ResolveVariableTypeTyped("$u", lines, textpos.Position{Line: 2, Character: 10}, parser.ParseFile(src, token.Latest)); !got.IsEmpty() {
		t.Errorf("$u = %q, want nothing", got.FullString())
	}
}
