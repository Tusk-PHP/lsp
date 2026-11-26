package checks

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestUnknownNamedArgument(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///lib.php", `<?php
namespace App;
function greet(string $name, int $times = 1) {}
function spread(...$rest) {}
class User {
    public function __construct(public string $name, public ?string $email = null) {}
    public static function make(string $name): static { return new static($name); }
    public function rename(string $name): void {}
}
class Magic { public function __call($n, $a) {} public function run(int $x) {} }
class Plain {}
`)
	source := `<?php
namespace App;
greet(name: 'a', times: 2);
greet(nmae: 'a');
greet('a', tmes: 3);
spread(anything: 1);
greet(...$args, times: 2);
greet($ok ? FOO : BAR);
greet(\strlen(string: 'x'));
$u = new User(name: 'a', emial: 'b');
$v = User::make(nam: 'a');
$u->rename(nme: 'b');
(new Magic())->run(y: 1);
new Plain(x: 1);
new static(x: 1);
greet(name: 'a', times: fn () => greet(bad: 1));
strlen(string: 'x');
`
	resolver := func(expr, _ string, _, _ int, _ *parser.FileNode) string {
		switch expr {
		case "$u":
			return `App\User`
		case "(new Magic())":
			return `App\Magic`
		}
		return ""
	}
	r := &UnknownNamedArgumentRule{TypeResolver: resolver}
	var got []string
	for _, f := range r.Check(parser.ParseFile(source, token.Latest), source, idx) {
		if f.Code != rule.UnknownNamedArgument || f.Severity != rule.SeverityError {
			t.Errorf("finding %+v", f)
		}
		got = append(got, f.Message)
	}
	want := []string{
		"Unknown named argument 'nmae' to greet()",
		"Unknown named argument 'tmes' to greet()",
		"Unknown named argument 'emial' to User()",
		"Unknown named argument 'nam' to make()",
		"Unknown named argument 'nme' to rename()",
		"Unknown named argument 'bad' to greet()",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%v\nwant\n%v", got, want)
	}
	if !slices.Equal(r.Codes(), []rule.Code{rule.UnknownNamedArgument}) {
		t.Errorf("codes %v", r.Codes())
	}
	if r.Check(parser.ParseFile("<?php greet('a');", token.Latest), "<?php greet('a');", idx) != nil {
		t.Error("a file without a colon has no named arguments to check")
	}
}

func TestCallWalkerEdges(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///lib.php", `<?php
function greet(string $name) {}
class Box { public function __construct(int $size) {} public static function of(int $size) {} }
`)
	source := `<?php
class Lonely { function f() { parent::of(nope: 1); new parent(nope: 1); } }
self::of(nope: 1);
new self(nope: 1);
Box::$size;
Box::SIZE;
greet(/* why */ nmae: 'x');
greet(nmae: 'x'
`
	var got []string
	for _, f := range (&UnknownNamedArgumentRule{}).Check(parser.ParseFile(source, token.Latest), source, idx) {
		got = append(got, f.Message)
	}
	if !slices.Equal(got, []string{"Unknown named argument 'nmae' to greet()"}) {
		t.Errorf("parent:: without a parent, self outside a class and an unclosed call are skipped; a comment before the name isn't: %v", got)
	}
}

func TestArgumentCountIgnoresConstructorsButChecksCallsInsideCheckedCalls(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///lib.php", `<?php
function one(int $a) {}
function two(int $a, int $b) {}
class Box { public function __construct(int $size) {} }
`)
	source := "<?php\nnew Box();\none(two(1));\ntwo(1);\n"
	var got []int
	for _, f := range (&ArgumentCountRule{}).Check(parser.ParseFile(source, token.Latest), source, idx) {
		got = append(got, f.StartLine)
	}
	if !slices.Equal(got, []int{2, 3}) {
		t.Errorf("new Box() isn't checked; two(1) is, inside one(...) as on its own: %v", got)
	}
}

func duplicateDeclarationsIndex() *symbols.Index {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///a.php", `<?php
if (!function_exists('h')) { function h($a) {} }
class Dup { public function __construct($a) {} public static function make($a) {} public function run($a) {} }
`)
	idx.IndexFile("file:///b.php", `<?php
if (!function_exists('h')) { function h($b, $c) {} }
class Dup { public function __construct($b, $c) {} public static function make($b, $c) {} public function run($b, $c) {} }
`)
	return idx
}

func dupResolver(expr, _ string, _, _ int, _ *parser.FileNode) string {
	if expr == "$dup" {
		return "Dup"
	}
	return ""
}

func TestUnknownNamedArgumentWithSeveralDeclarations(t *testing.T) {
	idx := duplicateDeclarationsIndex()
	source := `<?php
h(a: 1);
h(c: 1);
h(z: 1);
new Dup(a: 1);
new Dup(z: 1);
Dup::make(b: 1);
Dup::make(z: 1);
$dup->run(a: 1);
$dup->run(z: 1);
`
	var got []int
	for _, f := range (&UnknownNamedArgumentRule{TypeResolver: dupResolver}).Check(parser.ParseFile(source, token.Latest), source, idx) {
		got = append(got, f.StartLine)
	}
	if !slices.Equal(got, []int{3, 5, 7, 9}) {
		t.Errorf("a named argument one declaration accepts is fine; one no declaration accepts is reported: lines %v", got)
	}
}

func TestUnknownNamedArgumentWhenOneDeclarationForwardsOrLacksTheMethod(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///a.php", "<?php\nclass Dup { public function __call($n, $a) {} public function run($a) {} }\nclass Gap { public function run($a) {} }\n")
	idx.IndexFile("file:///b.php", "<?php\nclass Dup { public function run($b) {} }\nclass Gap {}\n")
	source := "<?php\n$dup->run(z: 1);\n$gap->run(z: 1);\n"
	resolver := func(expr, _ string, _, _ int, _ *parser.FileNode) string {
		return map[string]string{"$dup": "Dup", "$gap": "Gap"}[expr]
	}
	if got := (&UnknownNamedArgumentRule{TypeResolver: resolver}).Check(parser.ParseFile(source, token.Latest), source, idx); len(got) != 0 {
		t.Errorf("a declaration with __call, or without the method, may accept anything: %+v", got)
	}
}

func TestUnknownNamedArgumentUsesEachDeclarationsOwnTraits(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///t.php", "<?php\ntrait T { public function m($x) {} }\nclass P { public function m($a) {} }\n")
	idx.IndexFile("file:///a.php", "<?php\nclass Dup { use T; }\n")
	idx.IndexFile("file:///b.php", "<?php\nclass Dup extends P {}\n")
	source := "<?php\n$dup->m(a: 1);\n$dup->m(x: 1);\n$dup->m(z: 1);\n"
	var got []int
	for _, f := range (&UnknownNamedArgumentRule{TypeResolver: dupResolver}).Check(parser.ParseFile(source, token.Latest), source, idx) {
		got = append(got, f.StartLine)
	}
	if !slices.Equal(got, []int{3}) {
		t.Errorf("a parameter of the method one declaration takes from its trait or parent is accepted: lines %v", got)
	}
}
