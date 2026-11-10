package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A variable that may hold the value of several writes has all of their
// types, and none when any of them can't be typed.
func TestVariableAssignedInSeveralBranches(t *testing.T) {
	src := `<?php
namespace App;
class A { public function a(): void {} }
class B { public function b(): void {} }
class C { public function c(): void {} }
class Check {
    /** @phpstan-assert B $x */
    public static function isB($x): void {}
}
function run(bool $f, int $n, A $p, $untyped, array $list): void {
    if ($f) {
        $ifElse = new A();
    } else {
        $ifElse = new B();
    }
    echo $ifElse; // ifElse
    switch ($n) {
        case 1:
            $switched = new A();
            break;
        default:
            $switched = new B();
    }
    echo $switched; // switched
    $ifOnly = new A();
    if ($f) {
        $ifOnly = new B();
        echo $ifOnly; // ifOnly inside
    }
    echo $ifOnly; // ifOnly after
    $looped = new A();
    while ($f) {
        $looped = new B();
    }
    echo $looped; // looped
    $every = new C();
    if ($f) {
        $every = new A();
    } else {
        $every = new B();
    }
    echo $every; // every
    $same = new A();
    if ($f) {
        $same = new A();
    }
    echo $same; // same
    $mixed = new A();
    if ($f) {
        $mixed = $untyped;
    }
    echo $mixed; // mixed
    if ($f) {
        $p = new B();
    }
    echo $p; // param
    $asserted = new A();
    if ($f) {
        $asserted = new C();
    }
    Check::isB($asserted);
    echo $asserted; // asserted
    $nextTurn = new A();
    foreach ($list as $item) {
        echo $nextTurn; // nextTurn
        $nextTurn = new B();
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
				return r.ResolveVariableTypeTyped(v, lines, protocol.Position{Line: i, Character: strings.Index(l, v) + 2}, file).FullString()
			}
		}
		t.Fatalf("no line marked %q", marker)
		return ""
	}
	// Read after the branch first, then inside it, then after it again: the
	// remembered types mustn't cross between them.
	for _, c := range []struct{ v, marker, want string }{
		{"$ifOnly", "ifOnly after", `App\A|App\B`},
		{"$ifOnly", "ifOnly inside", `App\B`},
		{"$ifOnly", "ifOnly after", `App\A|App\B`},
		{"$ifElse", "ifElse", `App\A|App\B`},
		{"$switched", "switched", `App\A|App\B`},
		{"$looped", "looped", `App\A|App\B`},
		{"$every", "every", `App\A|App\B`},
		{"$same", "same", `App\A`},
		{"$mixed", "mixed", ``},
		{"$p", "param", `App\A|App\B`},
		{"$asserted", "asserted", `App\B`},
		{"$nextTurn", "nextTurn", `App\A|App\B`},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}

// Each write is typed where it is written: its right-hand side reads the
// variable's previous value, and one that's a statement of its own is read
// from its column, wherever it is on its line. Method chains need the completion provider's
// chain resolver, so internal/lsp covers those, loops included.
func TestWritesAreTypedWhereTheyAreWritten(t *testing.T) {
	src := `<?php
namespace App;
class Foo {}
class Bar {}
function run(bool $c, int $n, array $xs, $untyped): void {
    $note = new Foo();
    $original = clone $note;
    foreach ($xs as $x) {
        $note = clone $original;
    }
    echo $original; // original
    $h = new Foo();
    $h = [$h]; // own rhs
    $u = new Foo();
    if ($c) { $u = new Bar(); }
    echo $u; // inline write
    switch ($n) {
        case 1: $y = new Foo(); break;
        case 2: $y = new Bar(); break;
    }
    echo $y; // case writes
    $s = new Foo();
    $c && $s = new Bar();
    echo $s; // short circuit
    $m = new Foo();
    $k = 1; $m = new Bar();
    echo $m; // second statement on its line
    foreach ($xs as $word) {
        $word = trim($word, " ");
        echo $word; // self in loop, no class
    }
    $q = new Foo();
    $q ??= new Bar();
    echo $q; // coalescing assignment
    $front = 'a';
    $front = $front . 'date/';
    echo $front; // concatenation
    $copy = $original; // keeps $note
    echo $copy; // trailing comment
}
`
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	at := func(v, marker string) string {
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+marker) {
				return r.ResolveVariableTypeTyped(v, lines, protocol.Position{Line: i, Character: strings.LastIndex(l, v) + 2}, file).FullString()
			}
		}
		t.Fatalf("no line marked %q", marker)
		return ""
	}
	for _, c := range []struct{ v, marker, want string }{
		{"$original", "original", `App\Foo`},
		{"$h", "own rhs", `App\Foo`},
		{"$u", "inline write", `App\Foo|App\Bar`},
		{"$y", "case writes", `App\Foo|App\Bar`},
		{"$s", "short circuit", ``},
		{"$m", "second statement on its line", `App\Bar`},
		{"$word", "self in loop, no class", `string`},
		{"$q", "coalescing assignment", `App\Foo|App\Bar`},
		{"$front", "concatenation", `string`},
		{"$copy", "trailing comment", `App\Foo`},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}
