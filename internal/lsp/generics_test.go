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
