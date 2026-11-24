<p>inline</p>
<?php
namespace App\Models {
    use Foo\Bar as Baz, Qux;
    use Lib\{One, Two as Three};
    use function Lib\helper;
    #[Marker] const LIMIT = 10, OTHER = LIMIT * 2;
    declare(ticks=1) { $ticks = 1; }

    #[Entity(table: 'users')]
    /** A user. */
    abstract class User extends Model implements Countable, JsonSerializable
    {
        use HasName, HasEmail {
            HasName::label insteadof HasEmail;
            HasEmail::label as protected emailLabel;
        }
        final public const string TYPE = 'user';
        #[Column] public private(set) ?int $id = null, $other;
        public string $name { get => strtoupper($this->name); set(string $value) { $this->name = $value; } }
        abstract protected function rules(): array;
        #[Pure] public static function &make(#[Sensitive] public readonly int|string &...$args): static|null
        {
            static $cache = [], $hits;
            global $config;
            return new static(...$args);
        }
    }

    interface Countable extends \Countable, Sized { const MAX = 1; public function count(): int; }
    #[Mixin] trait HasName { public function label(): string { return $this->name; } }
    #[Backed] enum Status: string implements HasLabel { case Active = 'a'; const DEFAULT = self::Active; }
    #[Api] function helper(A&B $x, (A&B)|C $y): never { throw new \Exception('x'); }
}

namespace {
    if ($a) { echo 1, 2; } elseif ($b) { echo 3; } else { echo 4; }
    while ($i < 10) { $i++; continue 1; }
    do { --$i; } while ($i > 0);
    for ($i = 0, $j = 1; $i < 10; $i++) { break 1; }
    foreach ($items as $key => &$value) { unset($value); }
    switch ($s) { case 1: echo 'one'; default: echo 'other'; }
    try { risky(); } catch (A | B $e) { log($e); } finally { done(); }
    goto end;
    end: ;
    $closure = #[Bound] static function &(int $x = 1) use (&$total, $rate): ?int { return $x; };
    $arrow = #[Pure] fn&(int $x): int => $x * 2;
    $v = $$name . ${'x'} . "a $b {$c->d} ${e} $f[0]" . <<<EOT
        heredoc $g
        EOT . `ls`;
    $r = [1, 'k' => &$ref, ...$rest];
    [$p, [$q]] = list('a' => $s) = $pair;
    $x = isset($a, $b) && empty($c) || !$d instanceof E;
    $y = (int) -$z ** 2 + @f() - ~$w;
    $t = $cond ? $yes : $no ?: $other ?? $default;
    $o = $obj?->prop->method(named: 1, ...$more)::CONST;
    $s = Klass::$static . Klass::method() . Klass::{'dyn'}() . $obj->{$name}[] = 1;
    $n = new Foo(1) . new class(2) extends Bar { public $p; } . clone $obj . clone($obj, ['a' => 1]);
    $m = match ($v) { 1, 2 => 'low', default => 'high' };
    $e = eval('1') . print 'p' . include 'f.php' . __LINE__ . exit(0);
    $g = fn() => yield $k => $v;
    $h = fn() => yield from $gen;
    $pipe = $value |> strlen(...);
    (void) discard();
    $f = strlen(...);
}
__halt_compiler(); raw data
