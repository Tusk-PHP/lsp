package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestNarrowingReadFromTheTree(t *testing.T) {
	src := `<?php
namespace App;
class User {
    /** @phpstan-assert-if-true Admin $this */
    public function isAdmin(): bool { return false; }
}
class Admin extends User {}
class Guest extends User {}
class Other {}
/** @template T */
class Box {
    /** @phpstan-self-out Box<int> */
    public function fill(): void {}
}
/** @phpstan-assert-if-true Admin $x */
function is_admin($x): bool { return true; }
/** @phpstan-assert-if-true =Admin $x */
function is_exactly_admin($x): bool { return true; }
/** @phpstan-assert Admin $x */
function must_be_admin($x): bool { return true; }
/** @param-out Admin $x */
function settle(&$x): void {}
/**
 * @template T of object
 * @param class-string<T> $class
 * @phpstan-assert-if-true T $x
 */
function is_of(string $class, $x): bool { return true; }
class Holder {
    public function run(User|Other $x, User|Other $y, Box $box, $fn, $cls, int $n): void {
        if (is_admin($x)) {
            echo $x; // if true
        } else {
            echo $x; // else of if true
        }
        if (is_exactly_admin($x)) {
        } else {
            echo $x; // else of an exact assertion
        }
        if (is_admin($y)) {
            echo $x; // assertion on another variable
        }
        if ($x->isAdmin()) {
            echo $x; // assertion on this
        }
        if (is_of(Guest::class, $x)) {
            echo $x; // templated assertion
        }
        if (!$x instanceof User) {
        }
        echo $x; // empty guard
        switch (true) {
            case $x instanceof Admin:
            case $x instanceof Guest:
                echo $x; // fallthrough cases
                break;
            case $n > 1:
                echo $x; // case without a type test
                break;
            case $x instanceof Admin && $n:
                echo $x; // case with a conjunction
                break;
        }
        switch ($n) {
            case 1:
                echo $x; // switch on a value
                break;
        }
        switch (true) {
            case $n:
                echo $x; // case on a plain value
        }
        switch (true) {
            case $x instanceof Missing:
                echo $x; // case on an unknown class
        }
        if (must_be_admin($x)) {
            echo $x; // unconditional assertion in a condition
        }
        if ($x instanceof Admin) {
            $f = function () use ($x) { // captured in a use clause
            };
        }
        if ($x instanceof Admin) {
            $x = $box;
            echo $x; // reassigned inside the test
            if ($x instanceof Box) {
                echo $x; // tested again after the reassignment
            }
        }
        $check = fn () => $x instanceof Admin && $x; // arrow function
    }
    public function guards(User|Other $x, Box $box, $fn, $cls): void {
        settle($x);
        echo $x; // param out
        $box->fill();
        echo $box; // self out
        $fn($x);
        $x->missing();
        echo $x; // unresolved calls
    }
    public function thrown(User|Other $x): void {
        if (!$x instanceof User) {
            throw new \Exception();
        }
        echo $x; // guard that throws
    }
    public function asserted(User|Other $x, User|Other $y, User|Other $z, $cls): void {
        $this->assertInstanceOf(Admin::class, $x);
        echo $x; // assertInstanceOf on this
        self::assertInstanceOf(Guest::class, $y);
        echo $y; // static assertInstanceOf
        self::assertInstanceOf($cls, $z);
        echo $z; // assertInstanceOf of an unknown class
    }
    public function inCase(User|Other $x, int $n): void {
        switch ($n) {
            case 1:
                assert($x instanceof Admin);
                echo $x; // assert in a case
        }
    }
}
`
	typed, _ := assignedTypes(t, src)
	for _, c := range []struct{ v, marker, want string }{
		{"$x", "if true", `App\Admin`},
		{"$x", "else of if true", `App\User|App\Other`},
		{"$x", "else of an exact assertion", `App\User|App\Other`},
		{"$x", "assertion on another variable", `App\User|App\Other`},
		{"$x", "assertion on this", `App\Admin`},
		{"$x", "templated assertion", `App\Guest`},
		{"$x", "empty guard", `App\User|App\Other`},
		{"$x", "fallthrough cases", `App\Guest|App\Admin`},
		{"$x", "case without a type test", `App\User|App\Other`},
		{"$x", "case with a conjunction", `App\Admin`},
		{"$x", "switch on a value", `App\User|App\Other`},
		{"$x", "case on a plain value", `App\User|App\Other`},
		{"$x", "param out", `App\Admin`},
		{"$box", "self out", `App\Box<int>`},
		{"$x", "unresolved calls", `App\Admin`},
		{"$x", "guard that throws", `App\User`},
		{"$x", "assertInstanceOf on this", `App\Admin`},
		{"$y", "static assertInstanceOf", `App\Guest`},
		{"$z", "assertInstanceOf of an unknown class", `App\User|App\Other`},
		{"$x", "assert in a case", `App\Admin`},
		{"$x", "case on an unknown class", ``},
		{"$x", "unconditional assertion in a condition", `App\User|App\Other`},
		{"$x", "captured in a use clause", `App\Admin`},
		{"$x", "reassigned inside the test", `App\Box`},
		{"$x", "tested again after the reassignment", `App\Box`},
	} {
		if got := typed(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
	lines := SplitLines(src)
	line := markedLine(t, lines, "arrow function")
	pos := textpos.Position{Line: line, Character: strings.LastIndex(lines[line], "$x") + 1}
	idx := symbols.NewIndex()
	idx.IndexFile("file:///n.php", src)
	if got := NewResolver(idx).ResolveVariableTypeTyped("$x", lines, pos, parser.ParseFile(src, token.Latest)).FullString(); got != `App\Admin` {
		t.Errorf("$x after && in an arrow function = %q, want App\\Admin", got)
	}
}

func TestVariablesWithoutAnIndex(t *testing.T) {
	src := "<?php\nfunction run(): void {\n    $n = 1;\n    echo $n;\n}\n"
	lines := SplitLines(src)
	if got := NewResolver(nil).ResolveVariableTypeTyped("$n", lines, textpos.Position{Line: 3, Character: 10}, parser.ParseFile(src, token.Latest)).FullString(); got != "int" {
		t.Errorf("$n without an index = %q, want int", got)
	}
}

func TestNarrowingInABracedNamespaceAfterManyStatements(t *testing.T) {
	var b strings.Builder
	b.WriteString("<?php\nnamespace App {\nclass User {}\nclass Admin extends User {}\nfunction make(): User { return new User(); }\n$x = make();\n")
	for range maxNarrowingStatements + 2 {
		b.WriteString("echo 1;\n")
	}
	b.WriteString("assert($x instanceof Admin);\necho $x; // read\n}\n")
	typed, _ := assignedTypes(t, b.String())
	if got := typed("$x", "read"); got != `App\Admin` {
		t.Errorf("$x = %q, want App\\Admin", got)
	}
}

func TestNoWriteOutsideEveryScope(t *testing.T) {
	src := "<?php\n$x = 1;\n"
	r := NewResolver(symbols.NewIndex())
	lines := SplitLines(src)
	tree := r.syntaxTree(lines, parser.ParseFile(src, token.Latest))
	if _, ok := r.lastWrite("$x", lines, tree, textpos.Position{Line: 5}, uint32(len(src))); ok {
		t.Error("lastWrite found a write for a position outside the file")
	}
}
