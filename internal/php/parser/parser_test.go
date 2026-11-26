package parser

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/phpref"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/strict"
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
	"<?php new class {}->foo();",
	"<?php new class {}::C;",
	"<?php new class {}[\"x\"];",
	"<?php new class(1) {}->foo;",
	"<?php for ((void) $a; (void) $b; (void) $c);",
	"<?php for ($x, (void) $a;;);",
	"<?php for (;; $x, (void) $a);",
	"<?php switch ($a): ?>\n<?php case 1: endswitch;",
	"<?php switch ($a) { ?>\n<?php case 1: }",
	"<?php switch ($a) { case 1 ?>\n<?php }",
	"<?php class C { use T { foo as final bar; foo as final; foo as static; foo as abstract x; foo as readonly y; foo as public(set) z; foo as var; } }",
	"<?php clone();",
	"<?php clone(...);",
	"<?php clone($o,);",
	"<?php clone($o, ...);",
	"<?php class C { $a; }",
	"<?php 1[0];",
	"<?php $a++ ++;",
	"<?php [$a] .= 1;",
	"<?php $a =& new A;",
	"<?php function f((A&B) $x) {}",
	"<?php class C { public $a, $b { get => 1; } }",
	"<?php class C { public public $a; }",
	"<?php class C { public static static function f() {} }",
	"<?php class C { final final const X = 1; }",
	"<?php final final class C {}",
	"<?php function f(public public $x) {}",
	"<?php class C { function __construct(public public $x) {} }",
	"<?php class C { public private $a; }",
	"<?php class C { abstract final function f(); }",
	"<?php $x = new readonly readonly class {};",
	"<?php readonly readonly class C {}",
	"<?php \"a\"[0];",
	"<?php __LINE__[0];",
	"<?php A[0];",
	"<?php A::B[0];",
	"<?php (1)[0];",
	"<?php [1][0];",
	"<?php $a++--;",
	"<?php ++$a++;",
	"<?php (int) $a = 1;",
	"<?php [$a][0] = 1;",
	"<?php [$a] = 1;",
	"<?php [[$a]] += 1;",
	"<?php list($a) .= 1;",
	"<?php $a =& f();",
	"<?php $a =& $b->c();",
	"<?php $a =& A::b();",
	"<?php $a =& new class {};",
	"<?php function f((A&B)|null $x) {}",
	"<?php function f((A&B)|(C&D) $x) {}",
	"<?php function f(): (A&B) {}",
	"<?php class C { public (A&B) $x; }",
	"<?php class C { public $a { get => 1; } }",
	"<?php class C { public $a = 1, $b; }",
	"<?php f() = 1;",
	"<?php $a->b() = 1;",
	"<?php (1) = 2;",
	"<?php 'a' = 1;",
	"<?php A::B = 1;",
	"<?php A = 1;",
	"<?php $a ? $b : $c = 1;",
	"<?php for ((void) $a, $b;;);",
	"<?php for (; (void) $a, $b;);",
	"<?php for (;; (void) $a, $b);",
	"<?php for (;; (void) $a);",
	"<?php for ((void) $a;;);",
	"<?php for (; $b, (void) $a;);",
	"<?php class C { use T { foo as final bar; } }",
	"<?php class C { use T { foo as final; } }",
	"<?php class C { use T { foo as static; } }",
	"<?php class C { use T { foo as abstract x; } }",
	"<?php class C { use T { foo as readonly y; } }",
	"<?php class C { use T { foo as public(set) z; } }",
	"<?php class C { use T { foo as var; } }",
	"<?php class C { use T { foo as public final bar; } }",
	"<?php clone(...$a);",
	"<?php clone(o: $a);",
	"<?php clone($a, $b, $c);",
	"<?php clone($a)->b;",
	"<?php $x = new class {}->foo;",
	"<?php new class {};",
	"<?php new A()->b;",
	"<?php new A->b;",
	"<?php switch ($a): ?><?php case 1: endswitch;",
	"<?php switch ($a): ?> <?php case 1: endswitch;",
	"<?php switch ($a) { case 1 ?><?php }",
	"<?php switch ($a) { case 1: ?>x<?php break; }",
	"<?php if ($a): ?><?php endif ?>",
	"<?php $a = 1 ?><?php ;",
	"<?php declare(ticks=1) ?>",
	"<?php class C { public $a; public $b }",
	"<?php abstract class C { public function f() {} }",
	"<?php class C { public int $p; }",
	"<?php class C { public const X = 1; }",
	"<?php class C { function __construct(public int $p) {} }",
	"<?php class C { public $h { public get => 1; } }",
	"<?php interface I { public const X = 1; }",
	"<?php abstract class C { static function f() {} }",
	"<?php class C { static int $p; }",
	"<?php class C { static const X = 1; }",
	"<?php class C { function __construct(static int $p) {} }",
	"<?php class C { public $h { static get => 1; } }",
	"<?php interface I { static const X = 1; }",
	"<?php abstract class C { abstract function f() {} }",
	"<?php class C { abstract int $p; }",
	"<?php class C { abstract const X = 1; }",
	"<?php class C { function __construct(abstract int $p) {} }",
	"<?php class C { public $h { abstract get => 1; } }",
	"<?php interface I { abstract const X = 1; }",
	"<?php abstract class C { final function f() {} }",
	"<?php class C { final int $p; }",
	"<?php class C { final const X = 1; }",
	"<?php class C { function __construct(final int $p) {} }",
	"<?php class C { public $h { final get => 1; } }",
	"<?php interface I { final const X = 1; }",
	"<?php abstract class C { readonly function f() {} }",
	"<?php class C { readonly int $p; }",
	"<?php class C { readonly const X = 1; }",
	"<?php class C { function __construct(readonly int $p) {} }",
	"<?php class C { public $h { readonly get => 1; } }",
	"<?php interface I { readonly const X = 1; }",
	"<?php abstract class C { var function f() {} }",
	"<?php class C { var int $p; }",
	"<?php class C { var const X = 1; }",
	"<?php class C { function __construct(var int $p) {} }",
	"<?php class C { public $h { var get => 1; } }",
	"<?php interface I { var const X = 1; }",
	"<?php abstract class C { public(set) function f() {} }",
	"<?php class C { public(set) int $p; }",
	"<?php class C { public(set) const X = 1; }",
	"<?php class C { function __construct(public(set) int $p) {} }",
	"<?php class C { public $h { public(set) get => 1; } }",
	"<?php interface I { public(set) const X = 1; }",
	"<?php abstract final class C {}",
	"<?php final abstract class C {}",
	"<?php class C { final private function f() {} }",
	"<?php class C { public protected(set) protected(set) int $a; }",
	"<?php class C { public(set) private(set) int $a; }",
	"<?php class C { abstract abstract function f(); }",
	"<?php class C { private private const X = 1; }",
	"<?php new readonly final class {};",
	"<?php new final class {};",
	"<?php class C { static abstract function f(); }",
	"<?php class C { abstract $p; }",
	"<?php 1->b;",
	"<?php 1->b();",
	"<?php 1?->b;",
	"<?php 1::B;",
	"<?php 1::b();",
	"<?php 1::$b;",
	"<?php 1();",
	"<?php 1++;",
	"<?php 1 = 1;",
	"<?php 1 .= 1;",
	"<?php 1 =& $b;",
	"<?php 1.5[0];",
	"<?php 1.5->b;",
	"<?php 1.5->b();",
	"<?php 1.5?->b;",
	"<?php 1.5::B;",
	"<?php 1.5::b();",
	"<?php 1.5::$b;",
	"<?php 1.5();",
	"<?php 1.5++;",
	"<?php 1.5 = 1;",
	"<?php 1.5 .= 1;",
	"<?php 1.5 =& $b;",
	"<?php 'a'[0];",
	"<?php 'a'->b;",
	"<?php 'a'->b();",
	"<?php 'a'?->b;",
	"<?php 'a'::B;",
	"<?php 'a'::b();",
	"<?php 'a'::$b;",
	"<?php 'a'();",
	"<?php 'a'++;",
	"<?php 'a' .= 1;",
	"<?php 'a' =& $b;",
	"<?php \"a$b\"[0];",
	"<?php \"a$b\"->b;",
	"<?php \"a$b\"->b();",
	"<?php \"a$b\"?->b;",
	"<?php \"a$b\"::B;",
	"<?php \"a$b\"::b();",
	"<?php \"a$b\"::$b;",
	"<?php \"a$b\"();",
	"<?php \"a$b\"++;",
	"<?php \"a$b\" = 1;",
	"<?php \"a$b\" .= 1;",
	"<?php \"a$b\" =& $b;",
	"<?php <<<E\nx\nE\n[0];",
	"<?php <<<E\nx\nE\n->b;",
	"<?php <<<E\nx\nE\n->b();",
	"<?php <<<E\nx\nE\n?->b;",
	"<?php <<<E\nx\nE\n::B;",
	"<?php <<<E\nx\nE\n::b();",
	"<?php <<<E\nx\nE\n::$b;",
	"<?php <<<E\nx\nE\n();",
	"<?php <<<E\nx\nE\n++;",
	"<?php <<<E\nx\nE\n = 1;",
	"<?php <<<E\nx\nE\n .= 1;",
	"<?php <<<E\nx\nE\n =& $b;",
	"<?php `ls`[0];",
	"<?php `ls`->b;",
	"<?php `ls`->b();",
	"<?php `ls`?->b;",
	"<?php `ls`::B;",
	"<?php `ls`::b();",
	"<?php `ls`::$b;",
	"<?php `ls`();",
	"<?php `ls`++;",
	"<?php `ls` = 1;",
	"<?php `ls` .= 1;",
	"<?php `ls` =& $b;",
	"<?php A->b;",
	"<?php A->b();",
	"<?php A?->b;",
	"<?php A::B;",
	"<?php A::b();",
	"<?php A::$b;",
	"<?php A();",
	"<?php A++;",
	"<?php A .= 1;",
	"<?php A =& $b;",
	"<?php \\A\\B[0];",
	"<?php \\A\\B->b;",
	"<?php \\A\\B->b();",
	"<?php \\A\\B?->b;",
	"<?php \\A\\B::B;",
	"<?php \\A\\B::b();",
	"<?php \\A\\B::$b;",
	"<?php \\A\\B();",
	"<?php \\A\\B++;",
	"<?php \\A\\B = 1;",
	"<?php \\A\\B .= 1;",
	"<?php \\A\\B =& $b;",
	"<?php A::B->b;",
	"<?php A::B->b();",
	"<?php A::B?->b;",
	"<?php A::B::B;",
	"<?php A::B::b();",
	"<?php A::B::$b;",
	"<?php A::B();",
	"<?php A::B++;",
	"<?php A::B .= 1;",
	"<?php A::B =& $b;",
	"<?php __LINE__->b;",
	"<?php __LINE__->b();",
	"<?php __LINE__?->b;",
	"<?php __LINE__::B;",
	"<?php __LINE__::b();",
	"<?php __LINE__::$b;",
	"<?php __LINE__();",
	"<?php __LINE__++;",
	"<?php __LINE__ = 1;",
	"<?php __LINE__ .= 1;",
	"<?php __LINE__ =& $b;",
	"<?php [1]->b;",
	"<?php [1]->b();",
	"<?php [1]?->b;",
	"<?php [1]::B;",
	"<?php [1]::b();",
	"<?php [1]::$b;",
	"<?php [1]();",
	"<?php [1]++;",
	"<?php [1] = 1;",
	"<?php [1] .= 1;",
	"<?php [1] =& $b;",
	"<?php array(1)[0];",
	"<?php array(1)->b;",
	"<?php array(1)->b();",
	"<?php array(1)?->b;",
	"<?php array(1)::B;",
	"<?php array(1)::b();",
	"<?php array(1)::$b;",
	"<?php array(1)();",
	"<?php array(1)++;",
	"<?php array(1) = 1;",
	"<?php array(1) .= 1;",
	"<?php array(1) =& $b;",
	"<?php ($a)[0];",
	"<?php ($a)->b;",
	"<?php ($a)->b();",
	"<?php ($a)?->b;",
	"<?php ($a)::B;",
	"<?php ($a)::b();",
	"<?php ($a)::$b;",
	"<?php ($a)();",
	"<?php ($a)++;",
	"<?php ($a) = 1;",
	"<?php ($a) .= 1;",
	"<?php ($a) =& $b;",
	"<?php $a++[0];",
	"<?php $a++->b;",
	"<?php $a++->b();",
	"<?php $a++?->b;",
	"<?php $a++::B;",
	"<?php $a++::b();",
	"<?php $a++::$b;",
	"<?php $a++();",
	"<?php $a++++;",
	"<?php $a++ = 1;",
	"<?php $a++ .= 1;",
	"<?php $a++ =& $b;",
	"<?php ++$a[0];",
	"<?php ++$a->b;",
	"<?php ++$a->b();",
	"<?php ++$a?->b;",
	"<?php ++$a::B;",
	"<?php ++$a::b();",
	"<?php ++$a::$b;",
	"<?php ++$a();",
	"<?php ++$a = 1;",
	"<?php ++$a .= 1;",
	"<?php ++$a =& $b;",
	"<?php f()[0];",
	"<?php f()->b;",
	"<?php f()->b();",
	"<?php f()?->b;",
	"<?php f()::B;",
	"<?php f()::b();",
	"<?php f()::$b;",
	"<?php f()();",
	"<?php f()++;",
	"<?php f() .= 1;",
	"<?php f() =& $b;",
	"<?php new A()[0];",
	"<?php new A()->b();",
	"<?php new A()?->b;",
	"<?php new A()::B;",
	"<?php new A()::b();",
	"<?php new A()::$b;",
	"<?php new A()();",
	"<?php new A()++;",
	"<?php new A() = 1;",
	"<?php new A() .= 1;",
	"<?php new A() =& $b;",
	"<?php A::class[0];",
	"<?php A::class->b;",
	"<?php A::class->b();",
	"<?php A::class?->b;",
	"<?php A::class::B;",
	"<?php A::class::b();",
	"<?php A::class::$b;",
	"<?php A::class();",
	"<?php A::class++;",
	"<?php A::class = 1;",
	"<?php A::class .= 1;",
	"<?php A::class =& $b;",
	"<?php $a::B[0];",
	"<?php $a::B->b;",
	"<?php $a::B->b();",
	"<?php $a::B?->b;",
	"<?php $a::B::B;",
	"<?php $a::B::b();",
	"<?php $a::B::$b;",
	"<?php $a::B();",
	"<?php $a::B++;",
	"<?php $a::B = 1;",
	"<?php $a::B .= 1;",
	"<?php $a::B =& $b;",
	"<?php static::B[0];",
	"<?php static::B->b;",
	"<?php static::B->b();",
	"<?php static::B?->b;",
	"<?php static::B::B;",
	"<?php static::B::b();",
	"<?php static::B::$b;",
	"<?php static::B();",
	"<?php static::B++;",
	"<?php static::B = 1;",
	"<?php static::B .= 1;",
	"<?php static::B =& $b;",
	"<?php (1)->b;",
	"<?php (1)->b();",
	"<?php (1)?->b;",
	"<?php (1)::B;",
	"<?php (1)::b();",
	"<?php (1)::$b;",
	"<?php (1)();",
	"<?php (1)++;",
	"<?php (1) = 1;",
	"<?php (1) .= 1;",
	"<?php (1) =& $b;",
	"<?php $a =& 1;",
	"<?php [$a] = &$b;",
	"<?php [&$a] = $b;",
	"<?php list($a) = &$b;",
	"<?php $a =& $b ?? $c;",
	"<?php $a =& [1];",
	"<?php $a =& ($b);",
	"<?php ++f();",
	"<?php ++($a);",
	"<?php $a->b++;",
	"<?php [$a, [$b]] = $c;",
	"<?php [$a, list($b)] = $c;",
	"<?php list($a, [$b]) = $c;",
	"<?php foreach ($a as [$b, [$c]]) {}",
	"<?php [[$a] = $b] = $c;",
	"<?php ($a) += 1;",
	"<?php ((($a))) = 1;",
	"<?php ($a)[0] = 1;",
	"<?php ($a)->b = 1;",
	"<?php $a = &new A;",
	"<?php $a =& new A();",
	"<?php $a = (clone $b)->c;",
	"<?php function(){}();",
	"<?php fn() => 1;",
	"<?php match(1) { default => 1 }[0];",
	"<?php isset($a)[0];",
	"<?php exit()[0];",
	"<?php $a = 1 ?>\n<?php + 2;",
	"<?php if ($a): ?>x<?php endif; ?>",
	"<?php class C { public $a ?>\n<?php }",
	"<?php $b = (void) f();",
	"<?php (void) f(); (void) $a->b;",
	"<?php function f(int|string $a) {}",
	"<?php function f(): static {}",
	"<?php f(a: 1);",
	"<?php class A { function __construct(public $a) {} }",
	"<?php $a ?? throw new E;",
	"<?php fn() => throw new E;",
	"<?php throw new E; if ($a) throw $b;",
	"<?php function f($a,) {}",
	"<?php function () use ($a,) {};",
	"<?php try {} catch (E) {}",
	"<?php new (foo());",
	"<?php $a instanceof (foo());",
	"<?php \"a$b\"[0];",
	"<?php __LINE__[0];",
	"<?php FOO->x;",
	"<?php Foo::BAR::$x;",
	"<?php Foo::BAR->x;",
	"<?php Foo::BAR[0]; FOO[0]; 'abc'->x; [1]->x; 'f'();",
	"<?php #[A] function f() {}",
	"<?php use \\A\\B, C\\D; use \\E\\{F, G\\H}; try {} catch (\\A\\B | C\\D $e) {} class X extends \\A\\B implements C\\D { use \\T\\U { \\T\\U::m insteadof V; } public function f(\\A $a, ?\\B\\C $b): \\D { return new \\A\\B instanceof namespace\\C; } }",
	"<?php $a{0}; 'abc'{0}; foo(){0}; ($a){0}; $a->b{0}; Foo::$a{0}; new $a{0};",
	"<?php #[\nfunction g() {}",
	"<?php use C \\ D;",
	"<?php $a = (real) $b;",
	"<?php function match() {} match(1);",
	"<?php namespace\\foo(); Foo \\ Bar::x(); \\ /* c */ Foo::x();",
	"<?php namespace Foo \\ Bar;",
	"<?php $a = 1 . 2 + 3 << 4 . 5 - 6;",
	"<?php FOO{0};",
	"<?php __LINE__{0};",
	"<?php namespace \\Foo;",
	"<?php use namespace\\Foo;",
	"<?php clone(?); clone($a, ?);",
	"<?php FOO{0}; Foo::BAR{0}; __LINE__{0}; \"a$b\"{0};",
	"<?php use A\\{\\B};",
	"<?php use function namespace\\f;",
	"<?php use \\A\\{B\\C, function d};",
	"<?php namespace namespace\\Foo;",
	"<?php $this->\n    list;\n$a->\nif($x);\n$a->\nreturn;\n$a->\necho('x');\n$a->\nnew;\n$a->\nif($x) ? 1 : 2;\n$a->\nif($x)->b;\nFoo::\nclass;\nFoo::\nRETURN;\n$a->\nfunction();",
	"<?php $a?->\nforeach($b)[0];",
	"<?php $this->\nreturn $x;",
	"<?php $this->\nif ($x) {}",
	"<?php Foo::\nreturn $x;",
}

func TestParserAgreesWithPHPOnPrograms(t *testing.T) {
	sources := make(map[string]string, len(programs))
	for i, src := range programs {
		sources["p"+strconv.Itoa(i)] = src
	}
	for v := token.PHP74; v <= token.Latest; v++ {
		for _, src := range programs {
			checkTree(t, src, v)
		}
	}
	for _, php := range phpref.Versions(t) {
		t.Run(php.Version.String(), func(t *testing.T) { compareWithPHP(t, php, sources) })
	}
}

func checkTree(t *testing.T, src string, version token.Version) {
	t.Helper()
	if problem := treeProblem(src, version); problem != "" {
		t.Fatalf("%q on PHP %s: %s", src, version, problem)
	}
}

func treeProblem(src string, version token.Version) string {
	var toks []lexer.Token
	for _, tok := range lexer.Lex(src, version) {
		if tok.Kind != token.Whitespace && tok.Kind != token.Comment && tok.Kind != token.DocComment {
			toks = append(toks, tok)
		}
	}
	return fileProblem(src, Parse(src, version), toks)
}

func fileProblem(src string, file *ast.File, toks []lexer.Token) string {
	for _, e := range file.Errors {
		if strings.HasPrefix(e.Message, internalErrorPrefix) {
			return e.Message
		}
	}
	clean := len(file.Errors) == 0
	var problem string
	var check func(parent ast.Span, n ast.Node)
	check = func(parent ast.Span, n ast.Node) {
		b := n.Bounds()
		if b.Start > b.End || b.Start < parent.Start || b.End > parent.End {
			problem = fmt.Sprintf("%T [%d,%d) lies outside its parent [%d,%d)", n, b.Start, b.End, parent.Start, parent.End)
			return
		}
		if _, isFile := n.(*ast.File); clean && !isFile && b.Start < b.End {
			if p := spanProblem(toks, b); p != "" {
				problem = fmt.Sprintf("%T [%d,%d) %q %s", n, b.Start, b.End, b.Text(src), p)
				return
			}
		}
		prev := ast.Span{Start: b.Start, End: b.Start}
		ast.Children(n, func(c ast.Node) {
			if problem != "" {
				return
			}
			cb := c.Bounds()
			if cb.Start < prev.End {
				problem = fmt.Sprintf("in %T, %T [%d,%d) starts before its previous sibling ends [%d,%d)", n, c, cb.Start, cb.End, prev.Start, prev.End)
				return
			}
			prev = cb
			check(b, c)
		})
	}
	check(file.Span, file)
	return problem
}

func spanProblem(toks []lexer.Token, span ast.Span) string {
	first, _ := slices.BinarySearchFunc(toks, span.Start, func(tok lexer.Token, at uint32) int { return cmp.Compare(tok.Start, at) })
	if first == len(toks) || toks[first].Start != span.Start {
		return "doesn't start at a token"
	}
	var open []token.Kind
	for _, tok := range toks[first:] {
		if tok.End > span.End {
			return "doesn't end at a token"
		}
		switch tok.Kind {
		case token.LeftParen, token.LeftBracket, token.Attribute, token.LeftBrace, token.CurlyOpen, token.DollarOpenCurlyBraces:
			open = append(open, tok.Kind)
		case token.RightParen, token.RightBracket, token.RightBrace:
			if len(open) == 0 || closer(open[len(open)-1]) != tok.Kind {
				return "closes a bracket it doesn't open"
			}
			open = open[:len(open)-1]
		}
		if tok.End == span.End {
			if len(open) > 0 {
				return "leaves a bracket open"
			}
			return ""
		}
	}
	return "doesn't end at a token"
}

func closer(open token.Kind) token.Kind {
	switch open {
	case token.LeftParen:
		return token.RightParen
	case token.LeftBracket, token.Attribute:
		return token.RightBracket
	}
	return token.RightBrace
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
	for _, src := range []string{"<?php\n$a = \"x $b\n", "<?php\n$a = `x\n", "<?php\n$a = <<<EOT\nx\n", "<?php\n$a = 'x\n"} {
		errs := Parse(src, token.Latest).Errors
		if len(errs) != 1 || errs[0].Message != "unterminated string" || errs[0].Start != 11 {
			t.Errorf("%q: got %v", src, errs)
		}
	}
}

func TestTreesAreWellFormedOnTheCorpus(t *testing.T) {
	for path, src := range phpref.ReadAll(t, phpref.Corpus(t)) {
		for _, v := range []token.Version{token.PHP74, token.Latest} {
			if problem := treeProblem(src, v); problem != "" {
				t.Errorf("%s on PHP %s: %s", path, v, problem)
			}
		}
	}
}

func TestTreeIsWellFormedOnTheSyntaxFixture(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "ast", "testdata", "syntax.php"))
	if err != nil {
		t.Fatal(err)
	}
	checkTree(t, string(src), token.Latest)
}

func TestAnonymousClassArgsBelongToTheClass(t *testing.T) {
	src := "<?php new class(1) extends B {};"
	file := Parse(src, token.Latest)
	n := file.Stmts[0].(*ast.ExprStmt).Expr.(*ast.New)
	if n.Args != nil || n.Anonymous.Args == nil || n.Anonymous.Args.Text(src) != "(1)" {
		t.Fatalf("got New.Args %v, class args %v", n.Args, n.Anonymous.Args)
	}
}

func stmtTypes(stmts []ast.Stmt) string {
	names := make([]string, len(stmts))
	for i, s := range stmts {
		names[i] = fmt.Sprintf("%T", s)
	}
	return strings.Join(names, " ")
}

func TestMissingSemicolonKeepsTheCodeAfterIt(t *testing.T) {
	cases := []struct {
		src  string
		body func(*ast.File) []ast.Stmt
		want string
	}{
		{
			src:  "<?php\nuse Foo\\Bar\nclass A { public function f() {} }\nfunction g() {}\n",
			body: func(f *ast.File) []ast.Stmt { return f.Stmts },
			want: "*ast.Use *ast.ClassDecl *ast.FunctionDecl",
		},
		{
			src: "<?php\nclass C {\n    public function m() {\n        $this->x\n        if ($a) { b(); }\n        return 1;\n    }\n}\n",
			body: func(f *ast.File) []ast.Stmt {
				return f.Stmts[0].(*ast.ClassDecl).Members[0].(*ast.Method).Body.Stmts
			},
			want: "*ast.ExprStmt *ast.If *ast.Return",
		},
		{
			src:  "<?php\n$x =\nfunction f() {}\n$y =\nclass B {}\n$z = 1\nfinal class D {}\n",
			body: func(f *ast.File) []ast.Stmt { return f.Stmts },
			want: "*ast.ExprStmt *ast.FunctionDecl *ast.ExprStmt *ast.ClassDecl *ast.ExprStmt *ast.ClassDecl",
		},
		{
			src:  "<?php\nfoo()\nbar();\n",
			body: func(f *ast.File) []ast.Stmt { return f.Stmts },
			want: "*ast.ExprStmt *ast.ExprStmt",
		},
	}
	for _, c := range cases {
		file := Parse(c.src, token.Latest)
		if len(file.Errors) == 0 {
			t.Errorf("%q: expected an error", c.src)
		}
		if got := stmtTypes(c.body(file)); got != c.want {
			t.Errorf("%q: got %s, want %s (errors %v)", c.src, got, c.want, file.Errors)
		}
	}
	file := Parse("<?php\nuse Foo\\Bar\nclass A { public function f() {} }\nfunction g() {}\n", token.Latest)
	if class := file.Stmts[1].(*ast.ClassDecl); len(class.Members) != 1 {
		t.Errorf("the class keeps its method: %v", class.Members)
	}
}

func TestAKeywordAfterAnArrowOnTheNextLineStartsTheNextStatement(t *testing.T) {
	method := func(body string) string {
		return "<?php\nclass C {\n    public function m() {\n        " + body + "\n    }\n}\n"
	}
	methodBody := func(f *ast.File) []ast.Stmt {
		return f.Stmts[0].(*ast.ClassDecl).Members[0].(*ast.Method).Body.Stmts
	}
	cases := []struct {
		src     string
		version token.Version
		want    string
	}{
		{method("$this->\n        return $x;"), token.Latest, "*ast.ExprStmt *ast.Return"},
		{method("$this?->\n        echo 'a';"), token.Latest, "*ast.ExprStmt *ast.Echo"},
		{method("$this->\n        if ($x) { y(); }\n        return 1;"), token.Latest, "*ast.ExprStmt *ast.If *ast.Return"},
		{method("$this->\n        foreach ($a as $b) {}"), token.PHP80, "*ast.ExprStmt *ast.Foreach"},
		{method("$this->\n        IF ($x) { y(); }"), token.Latest, "*ast.ExprStmt *ast.If"},
		{method("$this->\n        try { y(); } finally {}"), token.Latest, "*ast.ExprStmt *ast.Try"},
		{method("$this-> // note\n        return $x;"), token.Latest, "*ast.ExprStmt *ast.Return"},
		{method("self::\n        return $x;"), token.Latest, "*ast.ExprStmt *ast.Return"},
		{method("parent::\n        self::m();"), token.Latest, "*ast.ExprStmt *ast.ExprStmt"},
		{method("$this->\n        return $x;"), token.PHP74, "*ast.ExprStmt *ast.Return"},
	}
	for _, c := range cases {
		file := Parse(c.src, c.version)
		if len(file.Errors) != 1 {
			t.Errorf("%q on PHP %s: want one error, got %v", c.src, c.version, file.Errors)
		}
		if got := stmtTypes(methodBody(file)); got != c.want {
			t.Errorf("%q on PHP %s: got %s, want %s (errors %v)", c.src, c.version, got, c.want, file.Errors)
		}
		checkTree(t, c.src, c.version)
	}
}

func TestAKeywordNamedMemberOnTheNextLineStaysAMember(t *testing.T) {
	cases := []struct {
		src     string
		version token.Version
		want    string
	}{
		{"<?php $this->\n    list;", token.Latest, "*ast.PropertyFetch"},
		{"<?php $this->\n    return;", token.Latest, "*ast.PropertyFetch"},
		{"<?php $this->\n    list = 1;", token.Latest, "*ast.Assign"},
		{"<?php $this->\n    if($x);", token.Latest, "*ast.MethodCall"},
		{"<?php $this->\n    if($x)->y();", token.Latest, "*ast.MethodCall"},
		{"<?php $this->\n    echo('x') . 'y';", token.Latest, "*ast.Binary"},
		{"<?php $this->\n    if($x){0};", token.PHP74, "*ast.ArrayDimFetch"},
		{"<?php Foo::\n    class;", token.Latest, "*ast.ClassConstFetch"},
		{"<?php Foo::\n    RETURN;", token.Latest, "*ast.ClassConstFetch"},
		{"<?php $this->return\n    ;", token.Latest, "*ast.PropertyFetch"},
	}
	for _, c := range cases {
		file := Parse(c.src, c.version)
		if len(file.Errors) != 0 {
			t.Errorf("%q on PHP %s: errors %v", c.src, c.version, file.Errors)
			continue
		}
		if got := fmt.Sprintf("%T", file.Stmts[0].(*ast.ExprStmt).Expr); got != c.want {
			t.Errorf("%q on PHP %s: got %s, want %s", c.src, c.version, got, c.want)
		}
	}
}

func TestAnUnterminatedSingleQuotedStringIsALiteral(t *testing.T) {
	src := "<?php\nconfig('database."
	file := Parse(src, token.Latest)
	arg := file.Stmts[0].(*ast.ExprStmt).Expr.(*ast.Call).Args.Args[0].Value
	if lit, ok := arg.(*ast.Literal); !ok || lit.Kind != token.EncapsedAndWhitespace || lit.Text(src) != "'database." {
		t.Fatalf("argument = %#v", arg)
	}
	checkTree(t, src, token.Latest)
}

func TestAMissingClassMemberKeepsTheClass(t *testing.T) {
	for _, src := range []string{"<?php\nFoo::", "<?php\nfunction f() {\n    Foo::\n}", "<?php\nfoo(Foo::"} {
		file := Parse(src, token.Latest)
		var fetch *ast.ClassConstFetch
		ast.Walk(file, func(n ast.Node) bool {
			if f, ok := n.(*ast.ClassConstFetch); ok {
				fetch = f
			}
			return true
		})
		if fetch == nil || fetch.Class.Bounds().Text(src) != "Foo" || !ast.IsMissing(fetch.Name) {
			t.Errorf("%q: got %#v", src, fetch)
		}
		if len(file.Errors) == 0 {
			t.Errorf("%q: expected an error", src)
		}
		checkTree(t, src, token.Latest)
	}
}

func TestAnUnclosedListReachesTheTokenAfterIt(t *testing.T) {
	cases := []struct {
		src      string
		unclosed bool
		end      string
	}{
		{"<?php\nf(1, ", true, "<?php\nf(1, "},
		{"<?php\nfunction g() {\n    f(1, \n}", true, "<?php\nfunction g() {\n    f(1, \n"},
		{"<?php\nf(1);", false, "<?php\nf(1)"},
	}
	for _, c := range cases {
		file := Parse(c.src, token.Latest)
		var args *ast.ArgList
		ast.Walk(file, func(n ast.Node) bool {
			if a, ok := n.(*ast.ArgList); ok {
				args = a
			}
			return true
		})
		if args.Unclosed != c.unclosed || int(args.End) != len(c.end) {
			t.Errorf("%q: unclosed %v, end %d; want %v, %d", c.src, args.Unclosed, args.End, c.unclosed, len(c.end))
		}
	}
	arrays := []struct {
		src      string
		unclosed bool
	}{
		{"<?php\n$a = ['x', ", true},
		{"<?php\n$a = array('x', ", true},
		{"<?php\n$a = ['x'];", false},
	}
	for _, c := range arrays {
		file := Parse(c.src, token.Latest)
		array := file.Stmts[0].(*ast.ExprStmt).Expr.(*ast.Assign).Value.(*ast.Array)
		if array.Unclosed != c.unclosed || c.unclosed && int(array.End) != len(c.src) {
			t.Errorf("%q: unclosed %v, end %d", c.src, array.Unclosed, array.End)
		}
	}
	for src, unclosed := range map[string]bool{"<?php\n$a['x'": true, "<?php\n$a[": true, "<?php\n$a['x'];": false} {
		file := Parse(src, token.Latest)
		dim := file.Stmts[0].(*ast.ExprStmt).Expr.(*ast.ArrayDimFetch)
		if dim.Unclosed != unclosed {
			t.Errorf("%q: unclosed %v", src, dim.Unclosed)
		}
	}
}

func TestDocCommentsSurviveAttributesAndModifiers(t *testing.T) {
	src := "<?php\n#[A]\n/** fn */\nfunction f(\n    /** Leading. */\n    string $a,\n    string $b = 'x' /** Trailing with a default. */,\n    int $c /** Trailing. */,\n    $d,\n    #[B] /** After attribute. */ $e\n) {}\nclass C {\n    /** Before attribute. */\n    #[Route('/c')]\n    public function index() {}\n    #[Route('/d')]\n    /** After attribute. */\n    public function show() {}\n    #[A] /** Prop. */ public int $p;\n}\n"
	file := Parse(src, token.Latest)
	if len(file.Errors) != 0 {
		t.Fatalf("errors: %v", file.Errors)
	}
	fn := file.Stmts[0].(*ast.FunctionDecl)
	text := func(s ast.Span) string { return s.Text(src) }
	if got := text(fn.Doc); got != "/** fn */" {
		t.Errorf("function doc %q", got)
	}
	want := []string{"/** Leading. */", "/** Trailing with a default. */", "/** Trailing. */", "", "/** After attribute. */"}
	for i, p := range fn.Params {
		if got := text(p.Doc); got != want[i] {
			t.Errorf("param %d doc %q, want %q", i, got, want[i])
		}
	}
	members := file.Stmts[1].(*ast.ClassDecl).Members
	for i, w := range []string{"/** Before attribute. */", "/** After attribute. */", "/** Prop. */"} {
		var doc ast.Span
		switch m := members[i].(type) {
		case *ast.Method:
			doc = m.Doc
		case *ast.Property:
			doc = m.Doc
		}
		if got := text(doc); got != w {
			t.Errorf("member %d doc %q, want %q", i, got, w)
		}
	}
}

func TestDeepNestingIsAnErrorNotACrash(t *testing.T) {
	for _, src := range []string{
		"<?php $a = " + strings.Repeat("(", 1_000_000) + "1;",
		"<?php " + strings.Repeat("$", 1_000_000) + "a;",
		"<?php " + strings.Repeat("{", 1_000_000),
		"<?php $a = " + strings.Repeat("[", 1_000_000),
	} {
		errs := Parse(src, token.Latest).Errors
		if len(errs) != 1 || errs[0].Message != "nesting is too deep to parse" {
			t.Errorf("%.20q: got %v", src, errs)
		}
	}
	if errs := Parse("<?php $a = "+strings.Repeat("(", 1000)+"1"+strings.Repeat(")", 1000)+";", token.Latest).Errors; len(errs) != 0 {
		t.Errorf("realistic nesting parses: %v", errs)
	}
}

func TestAPanicKeepsThePartialFileAndReportsIt(t *testing.T) {
	p := &parser{src: "<?php"}
	file := &ast.File{}
	p.guard(func() {
		file.Stmts = append(file.Stmts, &ast.Nop{})
		panic("boom")
	})
	if len(file.Stmts) != 1 || len(p.errors) != 1 || p.errors[0].Message != "internal parser error: boom" {
		t.Fatalf("got %v %v", file.Stmts, p.errors)
	}
}

func TestAPanicIsRaisedAgainInStrictMode(t *testing.T) {
	t.Setenv(strict.EnvVar, "1")
	defer func() {
		if r := recover(); r != "boom" {
			t.Fatalf("got %v, want the panic raised again", r)
		}
	}()
	p := &parser{src: "<?php"}
	p.guard(func() { panic("boom") })
	t.Fatal("guard swallowed the panic")
}

func TestTreeProblemReportsAPanic(t *testing.T) {
	file := &ast.File{Errors: []ast.Error{{Message: internalErrorPrefix + "boom"}}}
	if problem := fileProblem("<?php", file, nil); problem != internalErrorPrefix+"boom" {
		t.Fatalf("got %q", problem)
	}
}

func TestNestedCloneCallsParseInLinearTime(t *testing.T) {
	const depth = 40
	for _, closed := range []bool{true, false} {
		src := "<?php " + strings.Repeat("clone(", depth) + "$o"
		if closed {
			src += strings.Repeat(")", depth) + ";"
		}
		done := make(chan *ast.File, 1)
		go func() { done <- Parse(src, token.PHP85) }()
		var file *ast.File
		select {
		case file = <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("closed=%v: %d nested clone( calls did not parse within 10s", closed, depth)
		}
		if closed != (len(file.Errors) == 0) {
			t.Errorf("closed=%v: errors %v", closed, file.Errors)
		}
		clones := 0
		var operand ast.Expr = file.Stmts[0].(*ast.ExprStmt).Expr
		for {
			c, ok := operand.(*ast.Clone)
			if !ok {
				break
			}
			clones++
			operand = c.Operand
		}
		if _, isVar := operand.(*ast.Variable); clones != depth || !isVar {
			t.Errorf("closed=%v: %d nested clones around %T, want %d around a variable", closed, clones, operand, depth)
		}
	}
}

func TestNestedAttributedClosuresParseInLinearTime(t *testing.T) {
	const depth = 40
	for _, closed := range []bool{true, false} {
		src := "<?php " + strings.Repeat("#[A(static function () { ", depth)
		if closed {
			src += strings.Repeat("})] function () {};", depth)
		}
		done := make(chan *ast.File, 1)
		go func() { done <- Parse(src, token.PHP85) }()
		var file *ast.File
		select {
		case file = <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("closed=%v: %d nested attributed closures did not parse within 10s", closed, depth)
		}
		if closed != (len(file.Errors) == 0) {
			t.Errorf("closed=%v: errors %v", closed, file.Errors)
		}
		if !closed {
			continue
		}
		levels := 0
		stmts := file.Stmts
		for len(stmts) > 0 {
			stmt, ok := stmts[0].(*ast.ExprStmt)
			if !ok {
				break
			}
			outer, ok := stmt.Expr.(*ast.Closure)
			if !ok || len(outer.Attrs) != 1 || outer.Attrs[0].Attrs[0].Args == nil {
				break
			}
			inner, ok := outer.Attrs[0].Attrs[0].Args.Args[0].Value.(*ast.Closure)
			if !ok || !inner.Static {
				break
			}
			levels++
			stmts = inner.Body.Stmts
		}
		if levels != depth {
			t.Errorf("%d nested attributed closures, want %d", levels, depth)
		}
	}
}

func TestErrorMessagesAreForPeople(t *testing.T) {
	for _, src := range programs {
		for _, e := range Parse(src, token.Latest).Errors {
			if strings.Contains(e.Message, "T_") || strings.Contains(e.Message, "''") {
				t.Errorf("%q: %q names an internal token", src, e.Message)
			}
		}
	}
	errs := Parse("<?php function f(A) {}", token.Latest).Errors
	if len(errs) != 1 || errs[0].Message != "syntax error, unexpected ')', expected a variable" {
		t.Errorf("got %v", errs)
	}
}

func TestVersionedSyntaxMatchesPHP(t *testing.T) {
	never := token.Latest + 1
	cases := []struct {
		src    string
		okFrom token.Version
	}{
		{"<?php clone($o,);", token.PHP85},
		{"<?php clone($o, $p);", token.PHP85},
		{"<?php new class {}->foo();", token.PHP84},
		{"<?php new class(1) {}->foo;", token.PHP84},
		{"<?php for ((void) $a, $b; (void) $c, $d; (void) $e);", token.PHP85},
		{"<?php for (; $b, (void) $a;);", never},
		{"<?php switch ($a): ?>\n<?php case 1: endswitch;", token.PHP74},
		{"<?php switch ($a) { case 1 ?>\n<?php }", token.PHP74},
		{"<?php class C { use T { foo as final bar; foo as static; } }", token.PHP74},
		{"<?php function f(int|string $a) {}", token.PHP80},
		{"<?php function f(): static {}", token.PHP80},
		{"<?php f(a: 1);", token.PHP80},
		{"<?php class A { function __construct(public $a) {} }", token.PHP80},
		{"<?php $a ?? throw new E;", token.PHP80},
		{"<?php fn() => throw new E;", token.PHP80},
		{"<?php throw new E; if ($a) throw $b;", token.PHP74},
		{"<?php function f($a,) {}", token.PHP80},
		{"<?php function () use ($a,) {};", token.PHP80},
		{"<?php try {} catch (E) {}", token.PHP80},
		{"<?php new (foo());", token.PHP80},
		{"<?php $a instanceof (foo());", token.PHP80},
		{"<?php \"a$b\"[0];", token.PHP80},
		{"<?php __LINE__[0];", token.PHP80},
		{"<?php FOO->x;", token.PHP80},
		{"<?php Foo::BAR::$x;", token.PHP80},
		{"<?php Foo::BAR->x;", token.PHP80},
		{"<?php $o?->p;", token.PHP80},
		{"<?php Foo::BAR[0]; FOO[0]; 'abc'->x; [1]->x; 'f'();", token.PHP74},
		{"<?php #[A] function f() {}", token.PHP74},
		{"<?php use \\A\\B, C\\D; use \\E\\{F, G\\H}; try {} catch (\\A\\B | C\\D $e) {} class X extends \\A\\B implements C\\D { use \\T\\U { \\T\\U::m insteadof V; } public function f(\\A $a, ?\\B\\C $b): \\D { return new \\A\\B instanceof namespace\\C; } }", token.PHP74},
		{"<?php class C { $a; }", never},
		{"<?php 1[0];", never},
		{"<?php $a++ ++;", never},
		{"<?php [$a] .= 1;", never},
		{"<?php $a =& new A;", never},
		{"<?php ($a) = 1;", never},
		{"<?php function f((A&B) $x) {}", never},
		{"<?php class C { public $a, $b { get => 1; } }", never},
		{"<?php class C { public public $a; }", never},
		{"<?php abstract final class C {}", never},
		{"<?php class C { var static $a; }", never},
	}
	for _, c := range cases {
		for v := token.PHP74; v <= token.Latest; v++ {
			errs := Parse(c.src, v).Errors
			if ok := len(errs) == 0; ok != (v >= c.okFrom) {
				t.Errorf("%q on PHP %s: errors %v", c.src, v, errs)
			}
		}
	}
}

func TestRemovedSyntaxParsesUntilItsRemoval(t *testing.T) {
	cases := []struct {
		src             string
		firstOK, lastOK token.Version
	}{
		{"<?php $a{0}; 'abc'{0}; foo(){0}; ($a){0}; $a->b{0}; Foo::$a{0}; new $a{0};", token.PHP74, token.PHP83},
		{"<?php FOO{0}; Foo::BAR{0}; __LINE__{0}; \"a$b\"{0};", token.PHP80, token.PHP83},
		{"<?php $a = (real) $b;", token.PHP74, token.PHP74},
		{"<?php #[\nfunction g() {}", token.PHP74, token.PHP74},
		{"<?php use C \\ D;", token.PHP74, token.PHP74},
		{"<?php function match() {} match(1);", token.PHP74, token.PHP74},
		{"<?php namespace\\foo(); Foo \\ Bar::x(); \\ /* c */ Foo::x();", token.PHP74, token.PHP74},
		{"<?php namespace Foo \\ Bar;", token.PHP74, token.PHP74},
		{"<?php /* never closed", token.PHP74, token.PHP74},
	}
	for _, c := range cases {
		for v := token.PHP74; v <= token.Latest; v++ {
			if errs := Parse(c.src, v).Errors; (len(errs) == 0) != (c.firstOK <= v && v <= c.lastOK) {
				t.Errorf("%q on PHP %s: errors %v", c.src, v, errs)
			}
		}
	}
}

func TestNamesSpanTheirTokensOn74(t *testing.T) {
	src := "<?php new \\ A \\ B; new namespace\\C; D\\E::f();"
	var got []string
	ast.Walk(Parse(src, token.PHP74), func(n ast.Node) bool {
		if name, ok := n.(*ast.Name); ok {
			got = append(got, name.Kind.String()+" "+name.Text(src))
		}
		return true
	})
	want := []string{"T_NAME_FULLY_QUALIFIED \\ A \\ B", "T_NAME_RELATIVE namespace\\C", "T_NAME_QUALIFIED D\\E"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConcatenationBindsLikeAdditionOn74(t *testing.T) {
	src := "<?php $a = 1 . 2 + 3 << 4;"
	for v, want := range map[token.Version]string{token.PHP74: "((1 . 2) + 3) << 4", token.PHP80: "1 . ((2 + 3) << 4)"} {
		var root *ast.Binary
		ast.Walk(Parse(src, v), func(n ast.Node) bool {
			if b, ok := n.(*ast.Binary); ok && root == nil {
				root = b
			}
			return true
		})
		if got := grouping(src, root); got != want {
			t.Errorf("PHP %s: got %s, want %s", v, got, want)
		}
	}
}

func grouping(src string, e ast.Expr) string {
	b, ok := e.(*ast.Binary)
	if !ok {
		return e.Bounds().Text(src)
	}
	left, right := grouping(src, b.Left), grouping(src, b.Right)
	if _, nested := b.Left.(*ast.Binary); nested {
		left = "(" + left + ")"
	}
	if _, nested := b.Right.(*ast.Binary); nested {
		right = "(" + right + ")"
	}
	return left + " " + b.Op.Text() + " " + right
}

func TestNestedDestructuringIsAList(t *testing.T) {
	for _, src := range []string{"<?php [$a, [$b, [$c]]] = $d;", "<?php list($a, [$b]) = $d;", "<?php foreach ($a as [$b, ['k' => [$c]]]) {}"} {
		lists := 0
		ast.Walk(Parse(src, token.Latest), func(n ast.Node) bool {
			switch n.(type) {
			case *ast.List:
				lists++
			case *ast.Array:
				t.Errorf("%q: a nested pattern stayed an array", src)
			}
			return true
		})
		if lists < 2 {
			t.Errorf("%q: %d lists", src, lists)
		}
	}
}

func TestParenthesizedExpressionsAreMarked(t *testing.T) {
	src := "<?php $a = (1 ? 2 : 3) ? 4 : 5; $b = 1 ? 2 : 3 ? 4 : 5; $c = (($d)); $e = ($f)->g;"
	file := Parse(src, token.PHP83)
	if len(file.Errors) > 0 {
		t.Fatalf("errors: %v", file.Errors)
	}
	var marked []string
	ast.Walk(file, func(n ast.Node) bool {
		if e, ok := n.(ast.Expr); ok && e.Parenthesized() {
			marked = append(marked, e.Bounds().Text(src))
		}
		return true
	})
	want := []string{"1 ? 2 : 3", "$d", "$f"}
	if !slices.Equal(marked, want) {
		t.Errorf("parenthesized: got %q, want %q", marked, want)
	}
}

func TestAParenthesizedVariableIsNotAssignable(t *testing.T) {
	for _, src := range []string{"<?php ($a) = 1;", "<?php [$a] = [1]; ([$b]) = [2];"} {
		if file := Parse(src, token.PHP83); len(file.Errors) == 0 {
			t.Errorf("%q parsed without an error", src)
		}
	}
}

func TestAShebangLineIsNotInlineHTML(t *testing.T) {
	for _, tc := range []struct {
		src     string
		shebang string
		html    string
	}{
		{"#!/usr/bin/env php\n<?php echo 1;", "#!/usr/bin/env php\n", ""},
		{"#!php\r\n<?php echo 1;", "#!php\r\n", ""},
		{"#!php\n\n<?php echo 1;", "#!php\n", "\n"},
		{"#!php", "#!php", ""},
		{" #!php\n<?php echo 1;", "", " #!php\n"},
		{"<p>\n<?php echo 1;", "", "<p>\n"},
	} {
		file := Parse(tc.src, token.PHP83)
		if got := file.Shebang.Text(tc.src); got != tc.shebang {
			t.Errorf("%q: shebang %q, want %q", tc.src, got, tc.shebang)
		}
		html := ""
		if len(file.Stmts) > 0 {
			if h, ok := file.Stmts[0].(*ast.InlineHTML); ok {
				html = h.Text(tc.src)
			}
		}
		if html != tc.html {
			t.Errorf("%q: inline HTML %q, want %q", tc.src, html, tc.html)
		}
	}
}

func TestAMissingArgumentKeepsTheArgumentsAfterIt(t *testing.T) {
	src := "<?php f(/* none yet */, $b, c: $c);"
	file := Parse(src, token.Latest)
	args := file.Stmts[0].(*ast.ExprStmt).Expr.(*ast.Call).Args.Args
	if len(args) != 3 {
		t.Fatalf("got %d arguments, want 3", len(args))
	}
	if missing, ok := args[0].Value.(*ast.BadExpr); !ok || missing.Start != missing.End {
		t.Errorf("first argument = %#v, want an empty bad expression", args[0].Value)
	}
	if got := args[1].Text(src); got != "$b" {
		t.Errorf("second argument = %q, want $b", got)
	}
	if got := args[2].Text(src); got != "c: $c" {
		t.Errorf("third argument = %q, want c: $c", got)
	}
	if len(file.Errors) != 1 || file.Errors[0].Message != "syntax error, unexpected ',', expected an expression" {
		t.Errorf("errors = %v, want only the missing argument", file.Errors)
	}
}
