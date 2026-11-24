package parser

import (
	"strconv"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/phpref"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

var programs = []string{
	"",
	"<p>html only</p>",
	"<?php",
	"<?php echo 1 ?>\n<p><?= $a, $b ?></p><?php echo 2;",
	"<?php ;;{} { $a; }",
	"<?php namespace A\\B; use C\\D, E\\F as G; use function h\\i; use const J\\K; use L\\{M, N as O, function p, const Q}; use function R\\{s, t};",
	"<?php namespace A { function f() {} } namespace { echo 1; }",
	"<?php declare(strict_types=1); declare(ticks=1) { $a; } declare(ticks=1): $b; enddeclare;",
	"<?php const A = 1, B = A + 1;",
	"<?php function &f(int $a = 1, ?string &$b = null, int|string ...$c): ?array { return [$a]; } function g(): static|null {}",
	"<?php function f(A&B $x, (A&B)|null $y, array $z, callable $c): never {}",
	"<?php abstract class A extends B implements C, D { use T1, T2 { T1::m insteadof T2; T2::m as protected n; m as o; p as private; } const X = 1, Y = 2; public const Z = 3; final protected static ?int $a = 1, $b; var $c; abstract public function f(); public function &g(): static { return $this; } public function __construct(private readonly int $x, protected ?A $y = null) {} }",
	"<?php final readonly class R {} #[A, B(1, named: 2)] #[C] class D {} interface I extends J, K { const X = 1; public function f(): void; } trait T { public static function s() {} }",
	"<?php enum Suit: string implements HasLabel { use T; case Hearts = 'H'; case Spades = 'S'; const Wild = self::Spades; public function label(): string { return match($this) { self::Hearts, self::Spades => 'x', default => 'y' }; } } enum Plain { case A; }",
	"<?php class C { const string NAME = 'c'; public int $x { get => $this->x * 2; set(int $v) { $this->x = $v; } } public private(set) string $y = ''; public function __construct(public protected(set) int $z = 0) {} public $h { final get { return 1; } } }",
	"<?php if ($a) b(); elseif ($c) d(); else if ($e) f(); else { g(); } if ($a): b(); elseif ($c): d(); else: e(); endif;",
	"<?php while ($a) { break; continue 2; } while ($a): b(); endwhile; do { $a--; } while ($a > 0); for ($i = 0, $j = 1; $i < 10; $i++, $j++) {} for (;;): endfor; for (;;);",
	"<?php foreach ($a as $v) {} foreach ($a as $k => &$v) {} foreach ($a as [$x, [$y, $z]]) {} foreach ($a as list('k' => $q)): endforeach;",
	"<?php switch ($a) { case 1: case 2; b(); break; default: c(); } switch ($a): case 1: endswitch; switch ($a) {}",
	"<?php try { a(); } catch (A | B $e) { b(); } catch (C) {} finally { c(); } try {} finally {}",
	"<?php global $a, $$b; static $c = 1, $d; unset($a, $b[1]); goto end; end: echo 'x'; return; return $a;",
	"<?php $a = $b ?: $c ?? $d; $a ??= 1; $a .= 'x'; $a **= 2; $a = &$b; [$a, , $b] = $c; list(, $x) = $y; ['k' => $a, 'l' => &$b] = $c;",
	"<?php $a = !$b instanceof C; $a = -$b ** 2; $a = (int) $b + 1; $a = @f(); $a = ~$b; $a = +$b; $a = $b++ + ++$c - $d-- - --$e;",
	"<?php $a = $b and $c or $d xor $e; $x = 1 <=> 2; $y = $a || $b && $c | $d ^ $e & $f; $z = $a << 1 >> 2 . 'x' . 3 % 4 / 5 * 6;",
	"<?php $f = function ($a) use ($b, &$c): int { return 1; }; $g = static fn&(int $x): int => $x; $h = #[A] static function () {}; $i = #[B] fn() => 1;",
	"<?php $o->p->q(); $o?->p?->q(); $o->{$name}(); $o->$name; A::b(); A::$c; A::C; A::class; static::d(); $a::e(); A::{'f'}(); new A; new A(); new $a; new $a->b; new static; new (getClass())(); new class(1) extends B implements C { public $x; };",
	"<?php $a[0][] = 1; $b = 'str'[0]; $c = [1, 2][0]; $d = (fn() => 1)(); $e = $f(...); $g = strlen(...); $h = A::m(...); $i = f(...$args, key: 1); $j = f(a: 1, b: 2,);",
	"<?php echo \"a $b {$c->d} ${e} {$f['g']} $h[0] $i->j\", <<<EOT\n x $y {$z}\nEOT, <<<'N'\nraw\nN, `ls $dir`;",
	"<?php $a = isset($b, $c['d']); $e = empty($f); eval('1'); exit; exit(1); die('x'); print 'p'; include 'a.php'; require_once __DIR__ . '/b.php'; $g = clone $h;",
	"<?php function gen() { yield; yield 1; yield 'k' => 'v'; $x = yield; yield from other(); } throw new E(); $a = $b ?? throw new E();",
	"<?php $a = match(true) { $b > 1, $b < -1 => 'far', default => 'near', }; $c = __LINE__ . __FILE__ . __DIR__ . __FUNCTION__ . __CLASS__ . __TRAIT__ . __METHOD__ . __NAMESPACE__;",
	"<?php #[Attr] function f() {} #[Attr] interface I {} #[Attr] trait T {} #[Attr] enum E {} #[Attr] abstract class A {}",
	"<?php __halt_compiler(); trailing garbage { ( [",
	"<?php $x = $$a; $y = ${'b'}; $z = $$$c; $w = ${$d . 'e'};",
	"<?php function readonly() {} readonly();",
	"<?php $a = new A()->b(); $c = new A()::C; $d = new A()['x'];",
	"<?php $r = $a |> strlen(...) |> fn($x) => $x * 2; (void) f(); $c = clone($o, ['p' => 1]); #[A] const X = 1;",
	"<?php $a = (array) $b; $c = (object) $d; $e = (bool) $f; $g = (float) $h; $i = (string) $j; $k = (unset) $l;",
	"<?php $x = A::{$name};",
	"<?php $x = new readonly class {};",
	"<?php class P { public function __construct(final public int $x) {} }",
	"<?php class F { final $a; final const X = 1; }",
	"<?php if (1) {} else if (2) {} elseif (3): endif;",
	"<?php $a = 1 == 2 == 3;",
	"<?php $a = 1 < 2 > 3;",
	"<?php $a = $b instanceof $c instanceof D;",
	"<?php function f( {}",
	"<?php class { }",
	"<?php echo 1",
	"<?php $a = ;",
	"<?php $a = [1, , 2];",
	"<?php foreach ($a as &$k => $v) {}",
	"<?php try {}",
	"<?php }",
	"<?php class A { public function }",
	"<?php class A { 1 }",
	"<?php function f(int $x { $y }) {}",
	"<?php use A\\{B, C",
	"<?php $o->1;",
	"<?php A::1;",
	"<?php \"$a[\";",
	"<?php $$;",
	"<?php if ($a): elseif ($b) endif;",
	"<?php switch ($a) { foo; }",
	"<?php #[A] $x;",
	"<?php new;",
	"<?php $x = match($a) { 1 => 2",
	"<?php fn($a) $a;",
	"<?php enum E: { case A; }",
	"<?php class C { use T { m as; } }",
	"<?php class C { public $a { get } }",
	"<?php $a = function() use ($b { };",
	"<?php function f(): ? {}",
	"<?php interface I { const; }",
	"<?php echo \"${a b}\";",
	"<?php $x = (int $y);",
	"<?php declare(a=1, b=2) $c;",
	"<?php while (1) else;",
	"<?php $f = static $x;",
	"<?php function f(...) {}",
	"<?php class H { public $a { &get => $this->a; } }",
	"<?php 1 = 2;",
	"<?php $b = clone($a); $c = clone ($a)->d;",
	"<?php echo \"$a[-1] $a[$k] $a[key] ${a[1]} ${b}\";",
	"<?php if ($a): else b(); endif;",
	"<?php switch ($a) { case 1 b(); }",
	"<?php if ($a)",
	"<?php function f((A&B)&C $x) {}",
	"<?php class C { use T { m bad; } }",
	"<?php echo \"${a b}\"; echo \"$a[ ]\";",
	"<?php f(...); $o->m(...); f(1, ...); f(?, a: ?); f(?);",
	"<?php f(...$a); f(a: ...$b);",
	"<?php /* never closed\nclass A {}",
	"<?php /** never closed",
	"<?php /*/ a",
	"<?php function f(?string|null $x) {}",
	"<?php #[] function f() {} #[A,] function g() {}",
	"<?php #[A,,] function f() {}",
	"<?php $a = static($c);",
	"<?php $a = static;",
	"<?php $a = static::$b + static::C + static::f() + (static fn () => 1)();",
}

func TestParserAgreesWithPHPOnPrograms(t *testing.T) {
	sources := make(map[string]string, len(programs))
	for i, src := range programs {
		sources["p"+strconv.Itoa(i)] = src
	}
	for v := token.PHP80; v <= token.Latest; v++ {
		for _, src := range programs {
			checkTree(t, src, Parse(src, v))
		}
	}
	for _, php := range phpref.Versions(t) {
		t.Run(php.Version.String(), func(t *testing.T) { compareWithPHP(t, php, sources) })
	}
}

func checkTree(t *testing.T, src string, file *ast.File) {
	t.Helper()
	var check func(parent ast.Span, n ast.Node)
	check = func(parent ast.Span, n ast.Node) {
		b := n.Bounds()
		if b.Start > b.End || b.Start < parent.Start || b.End > parent.End {
			t.Fatalf("%q: %T [%d,%d) lies outside its parent [%d,%d)", src, n, b.Start, b.End, parent.Start, parent.End)
		}
		ast.Children(n, func(c ast.Node) { check(b, c) })
	}
	check(file.Span, file)
}

func TestPartialApplicationNeedsPHP86(t *testing.T) {
	src := "<?php f(1, ...); f(?, a: ?); $o->m(?);"
	if errs := Parse(src, token.PHP86).Errors; len(errs) != 0 {
		t.Fatalf("8.6 parses partial application: %v", errs)
	}
	errs := Parse(src, token.PHP85).Errors
	if len(errs) != 4 {
		t.Fatalf("8.5 reports each placeholder: %v", errs)
	}
	for _, e := range errs {
		if e.Message != "partial function application requires PHP 8.6" {
			t.Errorf("unexpected error %v", e)
		}
	}
}

func TestUnterminatedCommentIsAnError(t *testing.T) {
	errs := Parse("<?php\nif ($a) {\n/* never closed\nclass A {}", token.Latest).Errors
	if len(errs) != 1 || errs[0].Message != "unterminated comment" || errs[0].Start != 16 || errs[0].End != 18 {
		t.Fatalf("got %v", errs)
	}
}

func TestParseCodeStartsInPHP(t *testing.T) {
	file := ParseCode("class A {}", token.Latest)
	if len(file.Errors) != 0 || len(file.Stmts) != 1 {
		t.Fatalf("got %v %v", file.Stmts, file.Errors)
	}
	if _, ok := file.Stmts[0].(*ast.ClassDecl); !ok {
		t.Fatalf("got %T", file.Stmts[0])
	}
}

func TestUnterminatedStringPointsAtItsOpening(t *testing.T) {
	for _, src := range []string{"<?php\n$a = \"x $b\n", "<?php\n$a = `x\n", "<?php\n$a = <<<EOT\nx\n"} {
		errs := Parse(src, token.Latest).Errors
		if len(errs) != 1 || errs[0].Message != "unterminated string" || errs[0].Start != 11 {
			t.Errorf("%q: got %v", src, errs)
		}
	}
}
