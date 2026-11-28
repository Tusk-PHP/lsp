package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

const templateLiteralsSource = `<?php
namespace App;

class Item {}

/**
 * @template T
 * @param T $value
 * @param callable(T): void $each
 */
function each_one($value, callable $each) {}

/**
 * @template T of object
 * @param class-string<T> $class
 * @phpstan-assert T $value
 */
function assertIs($value, string $class): void {}

/**
 * @template T of int
 * @param T $v
 * @return T
 */
function sameInt($v) {}

/**
 * @template T of array-key
 * @param T $v
 * @return T
 */
function sameKey($v) {}

/**
 * @template T of int|string
 * @param T $v
 * @return T
 */
function sameIntOrString($v) {}

function probe($x) {
    each_one(new Item(), function ($item) { echo $item; });
    assertIs($x, Item::class);
    echo $x;
    $a = sameInt(5);
    $b = sameKey(5);
    $c = sameIntOrString(5);
    echo $a, $b, $c;
}
`

func TestTemplatesBoundAtTheCallReachClosuresAssertionsAndLiterals(t *testing.T) {
	r, file, lines := setupGenericsResolverFor(t, templateLiteralsSource)
	at := func(needle, v string) textpos.Position {
		line := lineContaining(t, lines, needle)
		return textpos.Position{Line: line, Character: strings.LastIndex(lines[line], v) + 1}
	}
	for _, c := range []struct{ v, line, want string }{
		{"$item", "echo $item;", "App\\Item"},
		{"$x", "echo $x;", "App\\Item"},
		{"$a", "echo $a, $b, $c;", "5"},
		{"$b", "echo $a, $b, $c;", "int"},
		{"$c", "echo $a, $b, $c;", "int"},
	} {
		if got := r.ResolveVariableTypeTyped(c.v, lines, at(c.line, c.v), file).FullString(); got != c.want {
			t.Errorf("%s at %q = %q, want %q", c.v, c.line, got, c.want)
		}
	}
}

func setupGenericsResolverFor(t *testing.T, src string) (*Resolver, *parser.FileNode, []string) {
	t.Helper()
	r, file, lines := setupBoxResolver(t, src)
	r.TypedChainResolver = func(req *Resolver, expr, _ string, _ textpos.Position, f *parser.FileNode) ResolvedType {
		return req.ExpressionType(expr, ExprContext{File: f})
	}
	return r, file, lines
}
