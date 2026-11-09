package lsp

import "testing"

// genericsCases check PHPStan's generics through the real server: what hover
// and completion show, and that unknown-member sees the same types. Each
// case follows an example from PHPStan's documentation
// (https://phpstan.org/blog/generics-in-php-using-phpdocs and
// https://phpstan.org/blog/generics-by-examples), laid out like the
// php-version cases: the cursor sits at "<|>".
var genericsCases = []phpVersionCase{
	// Step 0: foundations.
	{name: "foundations/@return wins over a native type when it names a template", completion: []string{"getBusName"}, src: `
interface StampInterface {}
class BusNameStamp implements StampInterface { public function getBusName(): string { return ''; } }
final class Envelope {
	/**
	 * @template TStamp of StampInterface
	 * @param class-string<TStamp> $stampFqcn
	 * @return TStamp|null
	 */
	public function last(string $stampFqcn): ?StampInterface { return null; }
}
function run(Envelope $e): void { $e->last(BusNameStamp::class)?-><|> }`},
	{name: "foundations/the bound template is checked like any other type", clean: true, src: `
interface StampInterface {}
class BusNameStamp implements StampInterface { public function getBusName(): string { return ''; } }
final class Envelope {
	/**
	 * @template TStamp of StampInterface
	 * @param class-string<TStamp> $stampFqcn
	 * @return TStamp|null
	 */
	public function last(string $stampFqcn): ?StampInterface { return null; }
}
function run(Envelope $e): ?string { return $e->last(BusNameStamp::class)?->getBusName(); }`},
	{name: "foundations/@phpstan-return wins over @return", completion: []string{"name"}, src: `
class Repo {
	/**
	 * @return array
	 * @phpstan-return list<Person>
	 */
	public function all(): array { return []; }
}
function run(Repo $r): void { foreach ($r->all() as $p) { $p-><|> } }`},
	{name: "foundations/a union keeps the members of every class", completion: []string{"info", "name"}, src: `
class Svc {
	/** @return Logger|Person */
	public function pick() { return new Logger(); }
}
function run(Svc $s): void { $s->pick()-><|> }`},
	{name: "foundations/a member of only one class of a union is no unknown member", clean: true, src: `
class Svc {
	/** @return Logger|Person */
	public function pick() { return new Logger(); }
}
function run(Svc $s): void { $s->pick()->info('x'); }`},
	{name: "foundations/diagnostics see a generic's type argument", diag: []string{"unknown-member"}, src: `
/** @template T */
class Box {
	/** @return T */
	public function get() { return null; }
}
class Svc {
	/** @return Box<Logger> */
	public function box() { return new Box(); }
}
function run(Svc $s): void { $s->box()->get()->nope(); }`},
	{name: "foundations/diagnostics see a generic's members", clean: true, src: `
/** @template T */
class Box {
	/** @return T */
	public function get() { return null; }
}
class Svc {
	/** @return Box<Logger> */
	public function box() { return new Box(); }
}
function run(Svc $s): void { $s->box()->get()->info('x'); }`},
	{name: "foundations/generic @var on a local variable", completion: []string{"info"}, src: `
/** @template T */
class Box {
	/** @return T */
	public function get() { return null; }
}
function load(): object { return new \stdClass(); }
function run(): void {
	/** @var Box<Logger> $box */
	$box = load();
	$box->get()-><|>
}`},
	{name: "foundations/generic @var on a property", completion: []string{"info"}, src: `
/** @template T */
class Box {
	/** @return T */
	public function get() { return null; }
}
class Svc {
	/** @var Box<Logger> */
	private Box $box;
	function go(): void { $this->box->get()-><|> }
}`},
	{name: "foundations/@phpstan-var wins over @var on a property", completion: []string{"info"}, src: `
/** @template T */
class Box {
	/** @return T */
	public function get() { return null; }
}
class Svc {
	/**
	 * @var Box
	 * @phpstan-var Box<Logger>
	 */
	private $box;
	function go(): void { $this->box->get()-><|> }
}`},

	// Step 1: templates on methods and functions.
	{name: "method templates/bound from a plain argument", completion: []string{"name"}, src: `
/**
 * @template T
 * @param T $a
 * @return T
 */
function identity($a) { return $a; }
function run(): void { identity(new Person())-><|> }`},
	{name: "method templates/bound from a variable argument", hover: []string{"Person"}, src: `
/**
 * @template T
 * @param T $a
 * @return T
 */
function identity($a) { return $a; }
function run(Person $p): void { $x = identity($p); echo $<|>x; }`},
	{name: "method templates/bound through a method on an object", completion: []string{"info"}, src: `
class Factory {
	/**
	 * @template T of object
	 * @param class-string<T> $class
	 * @return T
	 */
	public function make(string $class): object { return new $class(); }
}
function run(Factory $f): void { $f->make(Logger::class)-><|> }`},
	{name: "method templates/bound from an array argument", completion: []string{"name"}, src: `
/**
 * @template TKey of array-key
 * @template TValue
 * @param array<TKey, TValue> $items
 * @return TValue
 */
function firstOf(array $items) { return reset($items); }
function run(Person $p): void { firstOf([$p])-><|> }`},
	{name: "method templates/bound from a generic object argument", completion: []string{"info"}, src: `
/** @template T */
class Box {
	/** @param T $value */
	public function __construct($value) {}
}
/**
 * @template T
 * @param Box<T> $box
 * @return T
 */
function unbox(Box $box) { return null; }
function run(Logger $l): void { unbox(new Box($l))-><|> }`},
	{name: "method templates/bound from a later argument", completion: []string{"info"}, src: `
class Picker {
	/**
	 * @template T
	 * @param string $label
	 * @param T $value
	 * @return T
	 */
	public function pick(string $label, $value) { return $value; }
}
function run(Picker $p, Logger $l): void { $p->pick('a', $l)-><|> }`},
	{name: "method templates/bound from a named argument", completion: []string{"info"}, src: `
class Picker {
	/**
	 * @template T
	 * @param string $label
	 * @param T $value
	 * @return T
	 */
	public function pick(string $label = '', $value = null) { return $value; }
}
function run(Picker $p, Logger $l): void { $p->pick(value: $l)-><|> }`},
	{name: "method templates/constructor inference anywhere", completion: []string{"name"}, src: `
/** @template T */
class Box {
	/** @param T $value */
	public function __construct($value) {}
	/** @return T */
	public function get() { return null; }
}
function run(): void { (new Box(new Person()))->get()-><|> }`},
	{name: "method templates/constructor inference into a variable", hover: []string{"Box\\u003cPerson\\u003e $box"}, src: `
/** @template T */
class Box {
	/** @param T $value */
	public function __construct($value) {}
}
function run(): void { $box = new Box(new Person()); echo $b<|>ox; }`},
	{name: "method templates/an unbound template falls back to its bound", completion: []string{"name"}, src: `
class Maker {
	/**
	 * @template T of Person
	 * @return T
	 */
	public function make() { return new Person(); }
}
function run(Maker $m): void { $m->make()-><|> }`},
	{name: "method templates/an unbound class template falls back to its default", completion: []string{"info"}, src: `
/** @template T = Logger */
class Box {
	/** @return T */
	public function get() { return null; }
}
function run(Box $b): void { $b->get()-><|> }`},
}

func TestGenerics(t *testing.T) {
	for _, tc := range genericsCases {
		t.Run(tc.name, func(t *testing.T) {
			runPHPVersionCase(t, tc)
		})
	}
}

var inheritanceCases = []phpVersionCase{
	{name: "inheritance/@implements gives an interface its type arguments", completion: []string{"bark"}, src: `
class Dog { public function bark(): void {} }
/** @template T */
interface Collection {
	/** @return T */
	public function get();
}
/** @implements Collection<Dog> */
class DogCollection implements Collection {
	public function get() { return new Dog(); }
}
function run(DogCollection $dogs): void { $dogs->get()-><|> }`},
	{name: "inheritance/@extends through several levels", completion: []string{"bark"}, src: `
class Dog { public function bark(): void {} }
/** @template T */
abstract class Repository {
	/** @return T */
	public function find(int $id) { return null; }
}
/** @extends Repository<Dog> */
class DogRepository extends Repository {}
class ShelterRepository extends DogRepository {}
function run(ShelterRepository $r): void { $r->find(1)-><|> }`},
	{name: "inheritance/a generic child passes its template through", completion: []string{"bark"}, src: `
class Dog { public function bark(): void {} }
/** @template T */
abstract class Repository {
	/** @return T */
	public function find(int $id) { return null; }
}
/**
 * @template TEntity
 * @template-extends Repository<TEntity>
 */
class CachedRepository extends Repository {}
/** @param CachedRepository<Dog> $r */
function run($r): void { $r->find(1)-><|> }`},
	{name: "inheritance/@use above a trait use statement", completion: []string{"bark"}, src: `
class Dog { public function bark(): void {} }
/** @template T */
trait HoldsItems {
	/** @return T */
	public function first() { return null; }
}
class Kennel {
	/** @use HoldsItems<Dog> */
	use HoldsItems;
}
function run(Kennel $k): void { $k->first()-><|> }`},
	{name: "inheritance/unknown-member sees an inherited generic type", diag: []string{"unknown-member"}, src: `
class Dog { public function bark(): void {} }
/** @template T */
abstract class Repository {
	/** @return T */
	public function find(int $id) { return null; }
}
/** @extends Repository<Dog> */
class DogRepository extends Repository {}
function run(DogRepository $r): void { $r->find(1)->meow(); }`},
	{name: "inheritance/foreach over IteratorAggregate type arguments", completion: []string{"bark"}, src: `
class Dog { public function bark(): void {} }
/** @implements \IteratorAggregate<int, Dog> */
class Pack implements \IteratorAggregate {
	public function getIterator(): \Iterator { return new \ArrayIterator([]); }
}
function run(Pack $pack): void { foreach ($pack as $dog) { $dog-><|> } }`},
	{name: "builtins/ArrayIterator values", completion: []string{"bark"}, src: `
class Dog { public function bark(): void {} }
/** @param \ArrayIterator<int, Dog> $it */
function run(\ArrayIterator $it): void { $it->current()-><|> }`},
	{name: "builtins/ArrayObject in foreach", completion: []string{"bark"}, src: `
class Dog { public function bark(): void {} }
/** @param \ArrayObject<int, Dog> $dogs */
function run(\ArrayObject $dogs): void { foreach ($dogs as $dog) { $dog-><|> } }`},
	{name: "builtins/SplObjectStorage data", completion: []string{"bark"}, src: `
class Dog { public function bark(): void {} }
/** @param \SplObjectStorage<Person, Dog> $s */
function run(\SplObjectStorage $s, Person $p): void { $s->offsetGet($p)-><|> }`},
	{name: "builtins/WeakMap values by offset", completion: []string{"bark"}, src: `
class Dog { public function bark(): void {} }
/** @param \WeakMap<Person, Dog> $map */
function run(\WeakMap $map, Person $p): void { $dog = $map[$p]; $dog-><|> }`},
}

func TestGenericsInheritance(t *testing.T) {
	for _, tc := range inheritanceCases {
		t.Run(tc.name, func(t *testing.T) {
			runPHPVersionCase(t, tc)
		})
	}
}

var iterableCases = []phpVersionCase{
	{name: "iterables/list<T> in foreach", completion: []string{"name"}, src: `
/** @return list<Person> */
function people(): array { return []; }
function run(): void { foreach (people() as $p) { $p-><|> } }`},
	{name: "iterables/non-empty-array<K, V> values", completion: []string{"name"}, src: `
/** @param non-empty-array<string, Person> $people */
function run(array $people): void { foreach ($people as $p) { $p-><|> } }`},
	{name: "iterables/iterable<K, V> keys", completion: []string{"name"}, src: `
/** @param iterable<Person, Logger> $map */
function run(iterable $map): void { foreach ($map as $person => $logger) { $person-><|> } }`},
	{name: "iterables/a Laravel-style collection in foreach", completion: []string{"name"}, src: `
/**
 * @template TKey of array-key
 * @template TValue
 * @extends \IteratorAggregate<TKey, TValue>
 */
interface Enumerable extends \IteratorAggregate {}
/**
 * @template TKey of array-key
 * @template TValue
 * @implements Enumerable<TKey, TValue>
 */
class Collection implements Enumerable {
	public function getIterator(): \Traversable { return new \ArrayIterator([]); }
}
/** @param Collection<int, Person> $people */
function run(Collection $people): void { foreach ($people as $p) { $p-><|> } }`},
	{name: "iterables/generator values in foreach", completion: []string{"name"}, src: `
/** @return \Generator<int, Person, string, Logger> */
function people(): \Generator { yield new Person(); return new Logger(); }
function run(): void { foreach (people() as $p) { $p-><|> } }`},
	{name: "iterables/generator getReturn()", completion: []string{"info"}, src: `
/** @return \Generator<int, Person, string, Logger> */
function people(): \Generator { yield new Person(); return new Logger(); }
function run(): void { $gen = people(); $gen->getReturn()-><|> }`},
	{name: "iterables/the value of yield is what the generator is sent", completion: []string{"info"}, src: `
/** @return \Generator<int, Person, Logger, void> */
function people(): \Generator {
	$logger = yield new Person();
	$logger-><|>
}`},
	{name: "iterables/the value at an array shape's key", completion: []string{"name"}, src: `
/** @param array{person: Person, count: int} $data */
function run(array $data): void { $data['person']-><|> }`},
	{name: "iterables/a shape's value assigned to a variable", completion: []string{"name"}, src: `
/** @param array{person: Person, count: int} $data */
function run(array $data): void { $p = $data['person']; $p-><|> }`},
	{name: "iterables/list{…} shapes by position", completion: []string{"info"}, src: `
/** @param list{Person, Logger} $pair */
function run(array $pair): void { $pair[1]-><|> }`},
	{name: "iterables/object shape properties complete", completion: []string{"person", "count"}, src: `
/** @param object{person: Person, count?: int} $data */
function run(object $data): void { $data-><|> }`},
	{name: "iterables/object shape property types", completion: []string{"name"}, src: `
/** @param object{person: Person, count?: int} $data */
function run(object $data): void { $data->person-><|> }`},
	{name: "iterables/value-of an array shape", completion: []string{"name"}, src: `
class Svc {
	/**
	 * @param array{a: Person, b: Person} $map
	 * @return value-of<array{a: Person, b: Person}>
	 */
	public function pick(array $map) { return $map['a']; }
}
function run(Svc $s): void { $s->pick([])-><|> }`},
	{name: "iterables/offset access T[K]", completion: []string{"name"}, src: `
class Svc {
	/**
	 * @template T of array{person: Person}
	 * @param T $data
	 * @return T['person']
	 */
	public function person(array $data) { return $data['person']; }
}
/** @param array{person: Person} $d */
function run(Svc $s, array $d): void { $s->person($d)-><|> }`},
	{name: "iterables/call-site variance reads as the type", completion: []string{"name"}, src: `
/** @template T */
class Box {
	/** @return T */
	public function get() { return null; }
}
/** @param Box<covariant Person> $box */
function run(Box $box): void { $box->get()-><|> }`},
	{name: "iterables/a wildcard type argument is mixed", clean: true, src: `
/** @template T */
class Box {
	/** @return T */
	public function get() { return null; }
}
/** @param Box<*> $box */
function run(Box $box): void { $box->get()->anything(); }`},
}

func TestGenericsIterables(t *testing.T) {
	for _, tc := range iterableCases {
		t.Run(tc.name, func(t *testing.T) {
			runPHPVersionCase(t, tc)
		})
	}
}

var callableCases = []phpVersionCase{
	{name: "callables/a closure's untyped parameter takes the callable's type", completion: []string{"name"}, src: `
class People {
	/** @param callable(Person, int): void $fn */
	public function each(callable $fn): void {}
}
function run(People $people): void { $people->each(function ($p, $i) { $p-><|> }); }`},
	{name: "callables/an arrow function's parameter takes a template bound by the receiver", completion: []string{"name"}, src: `
/** @template TValue */
class Collection {
	/**
	 * @template TMapValue
	 * @param callable(TValue): TMapValue $callback
	 * @return Collection<TMapValue>
	 */
	public function map(callable $callback) { return new Collection(); }
}
/** @param Collection<Person> $people */
function run(Collection $people): void { $people->map(fn ($p) => $p-><|>); }`},
	{name: "callables/templates bind through a callable's return", completion: []string{"info"}, src: `
/** @template TValue */
class Collection {
	/**
	 * @template TMapValue
	 * @param callable(TValue): TMapValue $callback
	 * @return Collection<TMapValue>
	 */
	public function map(callable $callback) { return new Collection(); }
	/** @return TValue|null */
	public function first() { return null; }
}
class Person2 { public Logger $logger; }
/** @param Collection<Person2> $people */
function run(Collection $people): void { $people->map(fn (Person2 $p) => $p->logger)->first()-><|> }`},
	{name: "callables/a closure's return type binds a template", completion: []string{"info"}, src: `
/**
 * @template T
 * @param \Closure(): T $factory
 * @return T
 */
function make(\Closure $factory) { return $factory(); }
function run(): void { make(function (): Logger { return new Logger(); })-><|> }`},
	{name: "callables/calling a Closure-typed variable", completion: []string{"info"}, src: `
/** @param \Closure(int): Logger $factory */
function run(\Closure $factory): void { $factory(1)-><|> }`},
	{name: "callables/calling an invokable object", completion: []string{"name"}, src: `
class Finder { public function __invoke(): Person { return new Person(); } }
function run(Finder $find): void { $find()-><|> }`},
	{name: "callables/@param-closure-this binds $this", completion: []string{"info"}, src: `
class Router {
	/** @param-closure-this Logger $routes */
	public function group(\Closure $routes): void {}
}
function run(Router $r): void { $r->group(function () { $this-><|> }); }`},
}

func TestGenericsCallables(t *testing.T) {
	for _, tc := range callableCases {
		t.Run(tc.name, func(t *testing.T) {
			runPHPVersionCase(t, tc)
		})
	}
}

var conditionalCases = []phpVersionCase{
	{name: "conditional/the branch the argument picks", completion: []string{"info"}, src: `
class Svc {
	/** @return ($asLogger is true ? Logger : Person) */
	public function get(bool $asLogger = false) { return new Person(); }
}
function run(Svc $s): void { $s->get(true)-><|> }`},
	{name: "conditional/the default value picks the other branch", completion: []string{"name"}, src: `
class Svc {
	/** @return ($asLogger is true ? Logger : Person) */
	public function get(bool $asLogger = false) { return new Person(); }
}
function run(Svc $s): void { $s->get()-><|> }`},
	{name: "conditional/the picked branch is checked", diag: []string{"unknown-member"}, src: `
class Svc {
	/** @return ($asLogger is true ? Logger : Person) */
	public function get(bool $asLogger = false) { return new Person(); }
}
function run(Svc $s): void { $s->get(false)->info('x'); }`},
	{name: "conditional/is not", completion: []string{"info"}, src: `
class Svc {
	/** @return ($value is not null ? Logger : Person) */
	public function get(?string $value) { return new Person(); }
}
function run(Svc $s): void { $s->get('a')-><|> }`},
	{name: "conditional/nested", completion: []string{"name"}, src: `
class Svc {
	/** @return ($a is int ? ($b is int ? Person : Logger) : Logger) */
	public function get($a, $b) { return new Person(); }
}
function run(Svc $s): void { $s->get(1, 2)-><|> }`},
	{name: "conditional/on a template", completion: []string{"info"}, src: `
class Container {
	/**
	 * @template T
	 * @param class-string<T>|string $id
	 * @return ($id is class-string<T> ? T : object)
	 */
	public function get(string $id) { return new \stdClass(); }
}
function run(Container $c): void { $c->get(Logger::class)-><|> }`},
	{name: "class-string/new $class", completion: []string{"info"}, src: `
function run(): void { $class = Logger::class; $obj = new $class(); $obj-><|> }`},
	{name: "class-string/$class::method()", completion: []string{"make"}, src: `
class Factory { public static function make(): Factory { return new Factory(); } }
/** @param class-string<Factory> $class */
function run(string $class): void { $class::<|> }`},
	{name: "class-string/new<T>", completion: []string{"info"}, src: `
class Container {
	/**
	 * @template T of object
	 * @param class-string<T> $class
	 * @return new<T>
	 */
	public function build(string $class) { return new $class(); }
}
function run(Container $c): void { $c->build(Logger::class)-><|> }`},
	{name: "mixin/a generic @mixin brings in members", completion: []string{"info"}, src: `
/**
 * @template T
 * @mixin T
 */
class Delegatee {}
/** @param Delegatee<Logger> $d */
function run(Delegatee $d): void { $d-><|> }`},
	{name: "mixin/a member from a generic @mixin has its type", completion: []string{"name"}, src: `
class Finder { public function person(): Person { return new Person(); } }
/**
 * @template T
 * @mixin T
 */
class Delegatee {}
/** @param Delegatee<Finder> $d */
function run(Delegatee $d): void { $d->person()-><|> }`},
}

func TestGenericsConditionalAndClassStrings(t *testing.T) {
	for _, tc := range conditionalCases {
		t.Run(tc.name, func(t *testing.T) {
			runPHPVersionCase(t, tc)
		})
	}
}

const assertPrelude = `
class Assert {
	/**
	 * @template ExpectedType of object
	 * @param class-string<ExpectedType> $expected
	 * @phpstan-assert =ExpectedType $actual
	 */
	public static function assertInstanceOf(string $expected, $actual): void {}
	/** @phpstan-assert !null $value */
	public static function assertNotNull($value): void {}
	/** @phpstan-assert-if-true Person $value */
	public static function isPerson($value): bool { return true; }
	/** @phpstan-assert-if-false Logger $value */
	public static function isNotLogger($value): bool { return true; }
}
`

var narrowingCases = []phpVersionCase{
	{name: "narrowing/@phpstan-assert after a call", completion: []string{"info"}, src: assertPrelude + `
function run(object $x): void { Assert::assertInstanceOf(Logger::class, $x); $x-><|> }`},
	{name: "narrowing/assertions through $this, as in PHPUnit", completion: []string{"name"}, src: assertPrelude + `
class Test extends Assert {
	function go(object $x): void { $this->assertInstanceOf(Person::class, $x); $x-><|> }
}`},
	{name: "narrowing/!null drops null", hover: []string{"Logger $l"}, src: assertPrelude + `
function run(?Logger $l): void { Assert::assertNotNull($l); echo $<|>l; }`},
	{name: "narrowing/an assertion is checked by unknown-member", clean: true, src: assertPrelude + `
function run(object $x): void { Assert::assertInstanceOf(Logger::class, $x); $x->info('a'); }`},
	{name: "narrowing/-if-true inside the if", completion: []string{"name"}, src: assertPrelude + `
function run(object $x): void { if (Assert::isPerson($x)) { $x-><|> } }`},
	{name: "narrowing/-if-true after an early return", completion: []string{"name"}, src: assertPrelude + `
function run(Person|Logger $x): void {
	if (!Assert::isPerson($x)) {
		return;
	}
	$x-><|>
}`},
	{name: "narrowing/-if-false in the else branch", completion: []string{"info"}, src: assertPrelude + `
function run(object $x): void {
	if (Assert::isNotLogger($x)) {
		echo 1;
	} else {
		$x-><|>
	}
}`},
	{name: "narrowing/an assertion ends with its block", hover: []string{"object $x"}, src: assertPrelude + `
function run(object $x): void {
	if (Assert::isPerson($x)) {
		echo 1;
	}
	echo $<|>x;
}`},
	{name: "narrowing/@param-out", completion: []string{"info"}, src: `
/** @param-out Logger $out */
function fill(&$out): void { $out = new Logger(); }
function run(): void { $out = null; fill($out); $out-><|> }`},
	{name: "narrowing/@phpstan-self-out", completion: []string{"name"}, src: `
/** @template T */
class Bag {
	/**
	 * @template TItem
	 * @param TItem $item
	 * @phpstan-self-out self<TItem>
	 */
	public function add($item): void {}
	/** @return T */
	public function first() { return null; }
}
function run(Bag $bag): void { $bag->add(new Person()); $bag->first()-><|> }`},
	{name: "narrowing/@phpstan-this-out", completion: []string{"info"}, src: `
/** @template T */
class Bag {
	/**
	 * @template TItem
	 * @param TItem $item
	 * @phpstan-this-out self<TItem>
	 */
	public function add($item): void {}
	/** @return T */
	public function first() { return null; }
}
function run(): void { $bag = new Bag(); $bag->add(new Logger()); $bag->first()-><|> }`},
}

func TestGenericsNarrowing(t *testing.T) {
	for _, tc := range narrowingCases {
		t.Run(tc.name, func(t *testing.T) {
			runPHPVersionCase(t, tc)
		})
	}
}

// fieldRunCases come from running the generics work over Laravel, Symfony,
// Magento 2 and WordPress: each is a false warning or a wrong type that run
// turned up.
var fieldRunCases = []phpVersionCase{
	{name: "field run/a call after an offset is followed", completion: []string{"getRetryCount"}, src: `
class RedeliveryStamp { public function getRetryCount(): int { return 0; } }
final class Envelope {
	/**
	 * @template TStamp of object
	 * @param class-string<TStamp> $class
	 * @return TStamp|null
	 */
	public function last(string $class): ?object { return null; }
}
/** @param list<Envelope> $envelopes */
function run(array $envelopes): void { $stamp = $envelopes[0]->last(RedeliveryStamp::class); $stamp-><|> }`},
	{name: "field run/a fully qualified PHPDoc type isn't read in the namespace", completion: []string{"getTimestamp"}, src: `
class DateTime { public function custom(): void {} }
interface Timezone {
	/** @return \DateTime */
	public function date();
}
function run(Timezone $tz): void { $tz->date()-><|> }`},
	{name: "field run/a chain goes on after a closure argument", completion: []string{"name"}, src: `
class PersonFactory {
	public static function new(): PersonFactory { return new PersonFactory(); }
	public function afterMaking(\Closure $fn): static { return $this; }
	public function createOne(): Person { return new Person(); }
}
function run(): void {
	$person = PersonFactory::new()
		->afterMaking(function (Person $p) {
			$p->name();
		})
		->createOne();
	$person-><|>
}`},
	{name: "field run/assertInstanceOf narrows without PHPUnit installed", noDiag: []string{"unknown-member"}, src: `
abstract class Shape {}
class Circle extends Shape { public function radius(): int { return 1; } }
class Factory {
	/**
	 * @template T of object
	 * @param class-string<T> $class
	 * @return T
	 */
	public function make(string $class): object { return new Circle(); }
}
class ShapeTest extends \PHPUnit\Framework\TestCase {
	function go(Factory $f): void {
		$shape = $f->make(Shape::class);
		$this->assertInstanceOf(Circle::class, $shape);
		echo $shape->radius();
	}
}`},
	{name: "field run/isset and unset of an undeclared property", clean: true, src: `
class Customer {
	public function reset(): void {
		if (isset($this->subscribed)) {
			unset($this->subscribed);
		}
		echo $this->label ?? '';
	}
}`},
	{name: "field run/AllowDynamicProperties", clean: true, src: `
#[\AllowDynamicProperties]
final class PostType {}
function run(PostType $type): void { $type->capabilities = []; echo count($type->capabilities); }`},
}

func TestGenericsFieldRun(t *testing.T) {
	for _, tc := range fieldRunCases {
		t.Run(tc.name, func(t *testing.T) {
			runPHPVersionCase(t, tc)
		})
	}
}
