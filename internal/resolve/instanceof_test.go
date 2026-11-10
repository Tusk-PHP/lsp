package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// `$x instanceof Foo` narrows $x where it holds: in the if block, after a
// guard that leaves, after assert(), and after && or || on the same line.
func TestInstanceofNarrowing(t *testing.T) {
	src := `<?php
namespace App;
use Other\Thing as Alias;
interface Shape {}
class Foo { public function fooOnly(): int { return 1; } }
class Bar {}
class Base {}
class Child extends Base {}
function run(object $o, Foo|Bar $u, Base $b, Foo $f, $m, object $g, object $h, object $a, Foo|Bar $w): void {
    if ($o instanceof Foo) {
        echo $o; // block
    }
    echo $o; // after block
    if ($u instanceof Foo) {
        echo $u; // union then
    } else {
        echo $u; // union else
    }
    if ($b instanceof Child) {
        echo $b; // subclass
    }
    if ($f instanceof Shape) {
        echo $f; // interface
    }
    if ($o instanceof Unknown) {
        echo $o; // unknown class
    }
    if (!$g instanceof Foo) {
        return;
    }
    echo $g; // guard
    if (!$h instanceof Foo || $h->fooOnly() > 1) {
        return;
    }
    echo $h; // guard or
    assert($a instanceof Bar);
    echo $a; // assert
    $ok = $w instanceof Foo && $w; // same line and
    $no = !$w instanceof Foo || $w; // same line or
    $q = $o instanceof Foo ? $o : null; // ternary
    if ($u instanceof Foo || $u instanceof Bar) {
        echo $u; // either
    }
    if ($o instanceof Alias) {
        echo $o; // alias
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	idx.IndexFile("file:///b.php", "<?php\nnamespace Other;\nclass Thing {}\n")
	r := NewResolver(idx)
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	at := func(v, marker string) string {
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+marker) {
				col := strings.LastIndex(l, v)
				return r.ResolveVariableTypeTyped(v, lines, protocol.Position{Line: i, Character: col + 1}, file).FullString()
			}
		}
		t.Fatalf("no line marked %q", marker)
		return ""
	}
	for _, c := range []struct{ v, marker, want string }{
		{"$o", "block", `App\Foo`},
		{"$o", "after block", `object`},
		{"$u", "union then", `App\Foo`},
		{"$u", "union else", `App\Bar`},
		{"$b", "subclass", `App\Child`},
		{"$f", "interface", `App\Foo&App\Shape`},
		{"$o", "unknown class", ``},
		{"$g", "guard", `App\Foo`},
		{"$h", "guard or", `App\Foo`},
		{"$a", "assert", `App\Bar`},
		{"$w", "same line and", `App\Foo`},
		{"$w", "same line or", `App\Foo`},
		{"$o", "ternary", `App\Foo`},
		{"$u", "either", `App\Foo|App\Bar`},
		{"$o", "alias", `Other\Thing`},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s at %q: got %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}

// The forms real code tests a variable in: loops, switch (true) and
// match (true) arms, ternaries over several lines, guards that do something
// before leaving, and a reassignment inside the block; and the places a test
// doesn't hold: a ternary's else branch, past a comma, after `||` in a
// ternary's condition.
func TestInstanceofNarrowingForms(t *testing.T) {
	src := `<?php
namespace App;
class Foo { public function inner(): Bar { return new Bar(); } }
class Bar {}
class Baz {}
function run(object $o, object $p, object $q, Foo|Bar $u, Foo|Bar $v, object $s, object $c, Foo|Bar $n): void {
    while ($o instanceof Foo) {
        echo $o; // while
    }
    switch (true) {
        case $p instanceof Foo:
            echo $p; // case
            break;
        case $p instanceof Bar:
        case $p instanceof Baz:
            if (true) {
                echo $p; // stacked cases
            }
            break;
    }
    $r = match (true) {
        $q instanceof Foo => $q, // match arm
        default => $q, // match default
    };
    $m = $u instanceof Foo
        ? $u // ternary next line
        : $u; // ternary else
    $w = foo($v instanceof Foo && $v, $v); // past comma
    $z = $n instanceof Foo || $n instanceof Bar ? $n : null; // or in ternary
    if (!$s instanceof Foo) {
        log('not a foo');
        return;
    }
    echo $s; // guard with statements
    if ($c instanceof Foo) {
        $c = $c->inner(); // reassigned
    }
    if ($o instanceof Bar) {
    } elseif ($o instanceof Foo) {
        echo $o; // elseif
    }
    switch (true) {
        case $q instanceof Foo && $q->ready():
            echo $q; // case with and
            break;
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	at := func(v, marker string) string {
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+marker) {
				code := l[:strings.Index(l, "//")]
				col := strings.LastIndex(code, v)
				return r.ResolveVariableTypeTyped(v, lines, protocol.Position{Line: i, Character: col + 1}, file).FullString()
			}
		}
		t.Fatalf("no line marked %q", marker)
		return ""
	}
	for _, c := range []struct{ v, marker, want string }{
		{"$o", "while", `App\Foo`},
		{"$p", "case", `App\Foo`},
		{"$p", "stacked cases", `App\Baz|App\Bar`},
		{"$q", "match arm", `App\Foo`},
		{"$q", "match default", `object`},
		{"$u", "ternary next line", `App\Foo`},
		{"$u", "ternary else", `App\Foo|App\Bar`},
		{"$v", "past comma", `App\Foo|App\Bar`},
		{"$n", "or in ternary", `App\Foo|App\Bar`},
		{"$s", "guard with statements", `App\Foo`},
		{"$c", "reassigned", `App\Foo`},
		{"$o", "elseif", `App\Foo`},
		{"$q", "case with and", `App\Foo`},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s at %q: got %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}
