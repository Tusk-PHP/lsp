package resolve

import "testing"

func TestReturnTypesResolveInTheMethodsEnvironment(t *testing.T) {
	r, file, _ := setupBoxResolver(t, `<?php
namespace App;

class Logger {}

final class Status {
    const ACTIVE = 'active';
}

/**
 * @template T
 */
class Box
{
    /** @return static<int> */
    public function ints() {}
    /**
     * @template C of object
     * @param class-string<C> $class
     * @return new<C>
     */
    public function build(string $class) {}
    /** @return Status::ACTIVE */
    public function status() {}
    /** @return Status::MISSING */
    public function missing() {}
    /** @return key-of<Unknown> */
    public function keys() {}
    /** @return Box<, Logger> */
    public function broken() {}
}
`)
	for expr, want := range map[string]string{
		"(new Box())->ints()":               `App\Box<int>`,
		"(new Box())->build(Logger::class)": `App\Logger`,
		"(new Box())->status()":             "string",
		"(new Box())->missing()":            "Status::MISSING",
		"(new Box())->keys()":               "key-of<Unknown>",
		"(new Box())->broken()":             `App\Box<, App\Logger>`,
	} {
		if got := r.ExpressionType(expr, ExprContext{File: file}).FullString(); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

func TestACollectionOfAnItemIsTheClassItsFactoryBuilds(t *testing.T) {
	r, file, _ := setupBoxResolver(t, `<?php
namespace App;

/**
 * @template TKey
 * @template TItem
 */
class Collection {}

/**
 * @template TKey
 * @template TItem
 * @extends Collection<TKey, TItem>
 */
class ProductCollection extends Collection {}

class Unrelated {}

class Product { public function newCollection(): ProductCollection {} }
class Plain { public function newCollection(): Collection {} }
class Untyped { public function newCollection() {} }
class Listed { public function newCollection(): array {} }
class Stranger { public function newCollection(): Unrelated {} }
class Bare { public $newCollection; }

class Shop
{
    /** @return Collection<int, Product> */
    public function products() {}
    /** @return Collection<int, Plain> */
    public function plains() {}
    /** @return Collection<int, Untyped> */
    public function untyped() {}
    /** @return Collection<int, Listed> */
    public function listeds() {}
    /** @return Collection<int, Stranger> */
    public function strangers() {}
    /** @return Collection<int, Bare> */
    public function bares() {}
    /** @return Collection<int, string> */
    public function names() {}
}
`)
	r.Index.SetCollectionFactories(map[string]string{`App\Collection`: "newCollection"})
	for expr, want := range map[string]string{
		"(new Shop())->products()":  `App\ProductCollection<int, App\Product>`,
		"(new Shop())->plains()":    `App\Collection<int, App\Plain>`,
		"(new Shop())->untyped()":   `App\Collection<int, App\Untyped>`,
		"(new Shop())->listeds()":   `App\Collection<int, App\Listed>`,
		"(new Shop())->strangers()": `App\Collection<int, App\Stranger>`,
		"(new Shop())->bares()":     `App\Collection<int, App\Bare>`,
		"(new Shop())->names()":     `App\Collection<int, string>`,
	} {
		if got := r.ExpressionType(expr, ExprContext{File: file}).FullString(); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
	if _, ok := r.Index.CollectionFactory(`App\ProductCollection`); ok {
		t.Error("only the registered collection class has a factory")
	}
}
