package parser

import "testing"

func TestDocBlockPrefixedTagsWin(t *testing.T) {
	doc := ParseDocBlock(`/**
 * @param array $items
 * @phpstan-param list<User> $items
 * @param int $count
 * @return array
 * @psalm-return list<int>
 * @phpstan-return list<User>
 * @var array
 * @phpstan-var array<int, User>
 */`)
	if doc == nil {
		t.Fatal("nil doc")
	}
	if len(doc.Params) != 2 || doc.Params[0].Type != "list<User>" || doc.Params[1].Type != "int" {
		t.Fatalf("params = %+v", doc.Params)
	}
	if doc.Return.Type != "list<User>" {
		t.Fatalf("return = %q", doc.Return.Type)
	}
	if got := doc.Tags["var"]; len(got) != 1 || got[0] != "array<int, User>" {
		t.Fatalf("var tags = %q", got)
	}
}

func TestDocBlockTemplatesWithBoundsAndDefaults(t *testing.T) {
	doc := ParseDocBlock(`/**
 * @template TKey of array-key
 * @template-covariant TValue of array<int, Foo>
 * @template T = string
 * @phpstan-template TModel of \Illuminate\Database\Eloquent\Model = \App\User
 */`)
	want := []DocTemplate{
		{Name: "TKey", Bound: "array-key"},
		{Name: "TValue", Bound: "array<int, Foo>"},
		{Name: "T", Default: "string"},
		{Name: "TModel", Bound: `\Illuminate\Database\Eloquent\Model`, Default: `\App\User`},
	}
	if len(doc.Templates) != len(want) {
		t.Fatalf("templates = %+v", doc.Templates)
	}
	for i, w := range want {
		got := doc.Templates[i]
		if got.Name != w.Name || got.Bound != w.Bound || got.Default != w.Default {
			t.Errorf("template %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestDocBlockInheritanceAndNarrowingTags(t *testing.T) {
	doc := ParseDocBlock(`/**
 * @extends Base<int, User>
 * @template-implements \IteratorAggregate<int, User>
 * @use HasItems<User>
 * @mixin T
 * @param-out int $count
 * @param-closure-this Builder $callback
 * @phpstan-assert !null $value
 * @phpstan-assert-if-true =User $this->user
 * @psalm-assert-if-false string $other
 * @phpstan-this-out self<TValue|TItem>
 * @assert (1, 2) == 3
 */`)
	if len(doc.Extends) != 1 || doc.Extends[0] != "Base<int, User>" {
		t.Errorf("extends = %q", doc.Extends)
	}
	if len(doc.Implements) != 1 || doc.Implements[0] != `\IteratorAggregate<int, User>` {
		t.Errorf("implements = %q", doc.Implements)
	}
	if len(doc.Uses) != 1 || doc.Uses[0] != "HasItems<User>" {
		t.Errorf("uses = %q", doc.Uses)
	}
	if len(doc.Mixins) != 1 || doc.Mixins[0] != "T" {
		t.Errorf("mixins = %q", doc.Mixins)
	}
	if len(doc.ParamOuts) != 1 || doc.ParamOuts[0].Type != "int" || doc.ParamOuts[0].Name != "$count" {
		t.Errorf("param-out = %+v", doc.ParamOuts)
	}
	if len(doc.ClosureThis) != 1 || doc.ClosureThis[0].Type != "Builder" || doc.ClosureThis[0].Name != "$callback" {
		t.Errorf("closure-this = %+v", doc.ClosureThis)
	}
	if doc.SelfOut != "self<TValue|TItem>" {
		t.Errorf("self-out = %q", doc.SelfOut)
	}
	want := []DocAssert{
		{Kind: AssertAlways, Type: "null", Target: "$value", Negated: true},
		{Kind: AssertIfTrue, Type: "User", Target: "$this->user", Equality: true},
		{Kind: AssertIfFalse, Type: "string", Target: "$other"},
	}
	if len(doc.Asserts) != len(want) {
		t.Fatalf("asserts = %+v", doc.Asserts)
	}
	for i, w := range want {
		if doc.Asserts[i] != w {
			t.Errorf("assert %d = %+v, want %+v", i, doc.Asserts[i], w)
		}
	}
}
