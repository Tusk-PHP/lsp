package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// genericsClosuresSource exercises, in the resolver itself, what the server
// tests in internal/lsp/generics_test.go check end to end: closures typed by
// the call they're passed to, @extends/@implements type arguments, type
// operators, conditional return types, array literals and type aliases.
const genericsClosuresSource = `<?php
namespace App;

class Logger { public function info(string $m): void {} }
class Person { public string $name = ''; public Logger $logger; }

class People {
    /** @param callable(Person, int): void $fn */
    public function each(callable $fn): void {}
}

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

class Router {
    /** @param-closure-this Logger $routes */
    public function group(\Closure $routes): void {}
}

/**
 * @template T
 * @param \Closure(): T $factory
 * @return T
 */
function make(\Closure $factory) { return $factory(); }

/** @template T */
interface Repository {
    /** @return T */
    public function find(int $id);
}
/** @template T */
abstract class BaseRepository {
    /** @return T */
    public function find(int $id) { return null; }
}
/**
 * @template TEntity
 * @extends BaseRepository<TEntity>
 */
abstract class CachedRepository extends BaseRepository {}
/** @extends CachedRepository<Person> */
class PersonRepository extends CachedRepository {}
class ShelterRepository extends PersonRepository {}
/** @implements Repository<Logger> */
class LoggerRepository implements Repository {
    public function find(int $id) {}
}
/**
 * @template TEntity
 * @implements Repository<TEntity>
 */
abstract class InterfaceRepository implements Repository {}
/** @extends InterfaceRepository<Person> */
class PeopleRepository extends InterfaceRepository {}

/**
 * @phpstan-type PersonMap array<string, Person>
 * @phpstan-type Shape array{person: Person, count: int}
 */
class Types {
    /** @return PersonMap */
    public function map() {}
    /** @return Shape */
    public function shape() {}
    /** @return value-of<Shape> */
    public function anyValue() {}
    /** @return key-of<PersonMap> */
    public function aKey() {}
    /** @return Shape['person'] */
    public function thePerson() {}
}

/** @phpstan-import-type PersonMap from Types */
class Importer {
    /** @return PersonMap */
    public function people() {}
}

class Finder {
    /**
     * @return ($many is true ? list<Person> : Person)
     */
    public function find(bool $many = false) {}
}

class Usage {
    public function run(People $people, Collection $col, Router $router, Finder $finder): void {
        $people->each(function ($p, $i) {
            echo $p, $i;
        });
        /** @var Collection<Person> $persons */
        $persons = $col;
        $persons->map(fn ($q) => $q);
        $names = $persons->map(fn (Person $q) => $q->logger)->first();
        $router->group(function () {
            echo $this;
        });
        $made = make(function (): Logger { return new Logger(); });
        $list = [new Person(), new Person()];
        $mixed = [new Person(), new Logger()];
        $one = $finder->find();
        $many = $finder->find(true);
        echo $names, $made, $list, $mixed, $one, $many;
    }
}
`

func setupGenericsClosuresResolver(t *testing.T) (*Resolver, *parser.FileNode, []string) {
	t.Helper()
	r, file, lines := setupBoxResolver(t, genericsClosuresSource)
	return r, file, lines
}

// A closure passed to a call takes its parameters' types, and $this, from
// the callable the call expects.
func TestClosureTypesFromTheCall(t *testing.T) {
	r, file, lines := setupGenericsClosuresResolver(t)
	at := func(needle, v string) protocol.Position {
		line := lineContaining(t, lines, needle)
		return protocol.Position{Line: line, Character: strings.LastIndex(lines[line], v) + 1}
	}
	for _, c := range []struct{ v, line, want string }{
		{"$p", "echo $p, $i;", "App\\Person"},                  // callable(Person, int)
		{"$i", "echo $p, $i;", "int"},                          // its second parameter
		{"$q", "$persons->map(fn ($q) => $q);", "App\\Person"}, // TValue of Collection<Person>
		{"$this", "echo $this;", "App\\Logger"},                // @param-closure-this
		{"$made", "echo $names", "App\\Logger"},                // \Closure(): T bound by the closure's return
	} {
		if got := r.ResolveVariableTypeTyped(c.v, lines, at(c.line, c.v), file).FullString(); got != c.want {
			t.Errorf("%s at %q = %q, want %q", c.v, c.line, got, c.want)
		}
	}
}

// Type arguments through @extends and @implements, type aliases and their
// imports, type operators, conditional return types and array literals.
func TestGenericExpressionTypes(t *testing.T) {
	r, file, _ := setupGenericsClosuresResolver(t)
	ctx := ExprContext{File: file}
	for expr, want := range map[string]string{
		"(new PersonRepository())->find(1)":  "App\\Person", // @extends through a generic parent
		"(new ShelterRepository())->find(1)": "App\\Person", // and one more plain level
		"(new LoggerRepository())->find(1)":  "App\\Logger", // @implements
		"(new PeopleRepository())->find(1)":  "App\\Person", // declared only on the parent's interface
		"(new Types())->map()":               "array<string, App\\Person>",
		"(new Types())->shape()":             "array{person: App\\Person, count: int}",
		"(new Types())->anyValue()":          "App\\Person|int",
		"(new Types())->aKey()":              "string",
		"(new Types())->thePerson()":         "App\\Person",
		"(new Importer())->people()":         "array<string, App\\Person>",
		"(new Finder())->find()":             "App\\Person",
		"(new Finder())->find(false)":        "App\\Person",
		"(new Finder())->find(true)":         "list<App\\Person>",
		"[new Person(), new Person()]":       "array<int, App\\Person>",
		"['a' => new Person(), 'b' => 1]":    "array{a: App\\Person, b: int}",
		"(new Collection())->map(fn (Person $p): Logger => $p->logger)->first()": "?App\\Logger",
	} {
		if got := r.exprType(expr, ctx).FullString(); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}
