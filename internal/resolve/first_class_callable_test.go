package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const firstClassCallableSource = `<?php
namespace App;
class Foo { public function fooOnly(): int { return 1; } }
/** @template T */
class Box {
    /** @param T $v */
    public function __construct($v) {}
    /** @return T */
    public function get() { return null; }
    public static function make(string $label): static { return new static(null); }
}
function twice(int $n): string { return ''; }
`

func firstClassCallableResolver(t *testing.T) (*Resolver, *parser.FileNode) {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Box.php", firstClassCallableSource)
	return NewResolver(idx), parser.ParseFile(firstClassCallableSource)
}

func TestFirstClassCallableExpressionType(t *testing.T) {
	r, file := firstClassCallableResolver(t)
	boxOfFoo := ResolvedType{FQN: "App\\Box", Params: []ResolvedType{{FQN: "App\\Foo"}}}
	ctx := ExprContext{File: file, Locals: map[string]ResolvedType{
		"$b": boxOfFoo,
		"$f": {FQN: "Closure", Callable: &CallableType{Params: []ResolvedType{{FQN: "int"}}, Return: &ResolvedType{FQN: "App\\Foo"}}},
		"$x": {FQN: "App\\Foo"},
	}}
	tests := []struct{ expr, want string }{
		{"$b->get(...)", "Closure(): App\\Foo"},
		{"$b?->get( ... )", "Closure(): App\\Foo"},
		{"Box::make(...)", "Closure(string): App\\Box"},
		{"twice(...)", "Closure(int): string"},
		{"\\App\\twice(...)", "Closure(int): string"},
		{"undefined_function(...)", "Closure"},
		{"$f(...)", "Closure(int): App\\Foo"},
		{"$b->get(...)->call($x)", "App\\Foo"},
		// Calls, not first-class callables.
		{"$b->get()", "App\\Foo"},
		{"twice(...$args)", "string"},
		{"$f(1)", "App\\Foo"},
	}
	for _, tc := range tests {
		if got := r.ExpressionType(tc.expr, ctx).FullString(); got != tc.want {
			t.Errorf("ExpressionType(%q) = %q, want %q", tc.expr, got, tc.want)
		}
	}
}

func TestFirstClassCallableVariable(t *testing.T) {
	src := firstClassCallableSource + `function run(): void {
    /** @var \Closure(int): Foo $first */
    $first = make_it();
    $r = $first(1);
    $again = $first(...);
    $len = twice(...);
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Box.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	end := protocol.Position{Line: len(lines) - 2}
	for name, want := range map[string]string{
		"$r":     "App\\Foo",
		"$again": "Closure(int): App\\Foo",
		"$len":   "Closure(int): string",
	} {
		if got := r.ResolveVariableTypeTyped(name, lines, end, file).FullString(); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if got := r.ResolveVariableType("$len", lines, end, file); got != "Closure" {
		t.Errorf("plain type of $len = %q, want Closure", got)
	}
}

func TestIsFirstClassCallable(t *testing.T) {
	for expr, want := range map[string]bool{
		"strlen(...)":           true,
		"$this->format( ... );": true,
		"Foo::bar(...)":         true,
		"f(...$args)":           false,
		"f($a, ...)":            false,
		"f(g(...))":             false,
		"f()":                   false,
		"...":                   false,
	} {
		if got := IsFirstClassCallable(expr); got != want {
			t.Errorf("IsFirstClassCallable(%q) = %v, want %v", expr, got, want)
		}
	}
}
