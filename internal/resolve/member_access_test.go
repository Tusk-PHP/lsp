package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const memberAccessSource = `<?php
namespace App;
class Box {}
class Lid {}
class Room {
    public static Box $SIZE;
    public Lid $LID;
    const LID = 'lid';
    const int SIZE = 2;
    public static Box $DEPTH;
    public function size(): Lid {}
}
`

func memberAccessResolver() (*Resolver, *parser.FileNode) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///room.php", memberAccessSource)
	return NewResolver(idx), parser.ParseFile(memberAccessSource, token.Latest)
}

func TestMemberAccessOfReadsTheAccessForm(t *testing.T) {
	for _, c := range []struct {
		called, static bool
		name           string
		want           MemberAccess
	}{
		{true, true, "size", CallAccess},
		{true, false, "size", CallAccess},
		{false, true, "SIZE", ConstantAccess},
		{false, true, "$SIZE", PropertyAccess},
		{false, false, "LID", PropertyAccess},
	} {
		if got := MemberAccessOf(c.called, c.static, c.name); got != c.want {
			t.Errorf("MemberAccessOf(%v, %v, %q) = %v, want %v", c.called, c.static, c.name, got, c.want)
		}
	}
}

func TestFindAccessedMemberMatchesTheKindTheAccessNames(t *testing.T) {
	r, _ := memberAccessResolver()
	for _, c := range []struct {
		name   string
		access MemberAccess
		want   symbols.SymbolKind
	}{
		{"SIZE", ConstantAccess, symbols.KindConstant},
		{"$SIZE", PropertyAccess, symbols.KindProperty},
		{"SIZE", PropertyAccess, symbols.KindProperty},
		{"LID", PropertyAccess, symbols.KindProperty},
		{"size", CallAccess, symbols.KindMethod},
	} {
		if m := r.FindAccessedMember(`App\Room`, c.name, c.access); m == nil || m.Kind != c.want {
			t.Errorf("FindAccessedMember(%s, %v) = %+v, want a %v", c.name, c.access, m, c.want)
		}
	}
	for _, name := range []string{"DEPTH", "size"} {
		if m := r.AccessedMemberOn(ResolvedType{FQN: `App\Room`}, name, ConstantAccess); m != nil {
			t.Errorf("Room::%s names a constant, and Room has none of that name: got %+v", name, m)
		}
	}
}

func TestStaticAccessTypesTheConstantOrTheProperty(t *testing.T) {
	r, file := memberAccessResolver()
	for expr, want := range map[string]string{`Room::SIZE`: "int", `Room::$SIZE`: `App\Box`} {
		if got := r.exprType(expr, ExprContext{File: file}).FullString(); got != want {
			t.Errorf("exprType(%s) = %q, want %q", expr, got, want)
		}
	}
}

func TestCallsReachForwardedScopesAndNestedMixins(t *testing.T) {
	idx := symbols.NewIndex()
	idx.SetCallForwards(map[string][]string{"Model": {"Builder<static>"}})
	idx.SetScopeConventions(map[string]symbols.ScopeConvention{"Builder": {Prefix: "scope", Attribute: "Scope"}})
	idx.IndexFile("file:///a.php", `<?php
/** @template T */
class Builder { const LIMIT = 1; public function where() { return $this; } }
class Model {}
class Category extends Model {
    public function scopeActive($query) {}
    #[Scope]
    public function popular($query) {}
}
/** @mixin Middle */
class Outer {}
/** @mixin Inner */
class Middle {}
class Inner { public function deep() {} }
`)
	r := NewResolver(idx)
	category := ResolvedType{FQN: "Category"}
	for _, name := range []string{"where", "active"} {
		if fwd, ok := r.ForwardedReceiver(category, name, CallAccess); !ok || fwd.FQN != "Builder" {
			t.Errorf("Category::%s() runs on the builder, got %+v, %v", name, fwd, ok)
		}
	}
	if _, ok := r.ForwardedReceiver(category, "active", PropertyAccess); ok {
		t.Error("a scope is reached only by a call")
	}
	if fwd, ok := r.ForwardedReceiver(category, "LIMIT", ConstantAccess); ok {
		t.Errorf("Category::LIMIT isn't forwarded to the builder's constant, got %+v", fwd)
	}
	for name, want := range map[string]string{"where": "Builder::where", "active": "Category::scopeActive"} {
		if m, owner := r.LookupMember(category, name, CallAccess); m == nil || m.FQN != want || owner.FQN != "Builder" {
			t.Errorf("LookupMember(Category, %s) = %+v on %s, want %s on the builder", name, m, owner.FQN, want)
		}
	}
	builder := ResolvedType{FQN: "Builder", Params: []ResolvedType{category}}
	if m := r.ScopeMethod(builder, "popular"); m == nil || m.FQN != "Category::popular" {
		t.Errorf("a #[Scope] method is a scope, got %+v", m)
	}
	if m, owner := r.FindMemberOn(ResolvedType{FQN: "Outer"}, "deep", CallAccess); m == nil || owner.FQN != "Inner" {
		t.Errorf("a method of a mixin's mixin: got %+v on %s", m, owner.FQN)
	}
}

func TestInvokableOffsetAndIterableObjectsAreTypedByTheirMethods(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", `<?php
interface Traversable {}
interface Iterator extends Traversable {}
interface IteratorAggregate extends Traversable {}
interface ArrayAccess {}
class Box {}
class Doubler { public function __invoke(int $x): Box {} }
class Shelf implements ArrayAccess {
    public function offsetGet($offset): Box {}
    public function offsetExists($offset): bool {}
    public function offsetSet($offset, $value): void {}
    public function offsetUnset($offset): void {}
}
class Crate implements IteratorAggregate {
    /** @return array<int, Box> */
    public function getIterator(): array {}
}
class Belt implements Iterator {
    public function current(): Box {}
    public function key(): string {}
    public function next(): void {}
    public function rewind(): void {}
    public function valid(): bool {}
}
`)
	r := NewResolver(idx)
	if got := r.InvokeType(ResolvedType{FQN: "Doubler"}).FQN; got != "Box" {
		t.Errorf("calling a Doubler gives %q, want Box", got)
	}
	if got := r.CallableOf(ResolvedType{FQN: "Doubler"}); got.Callable == nil || got.Callable.Return == nil || got.Callable.Return.FQN != "Box" {
		t.Errorf("Doubler(...) is a Closure returning Box, got %+v", got)
	}
	if got := r.OffsetType(ResolvedType{FQN: "Shelf"}, "'top'").FQN; got != "Box" {
		t.Errorf("$shelf['top'] is %q, want Box", got)
	}
	for class, wantKey := range map[string]string{"Crate": "int", "Belt": "string"} {
		k, v, ok := r.IterableTypes(ResolvedType{FQN: class})
		if !ok || k.FQN != wantKey || v.FQN != "Box" {
			t.Errorf("iterating a %s gives %s => %s (%v), want %s => Box", class, k.FQN, v.FQN, ok, wantKey)
		}
	}
}

func TestAConstantIsNotReachedThroughAMixin(t *testing.T) {
	const source = `<?php
namespace App;
class Box {}
/** @mixin Extra */
class Room {}
class Extra {
    const int ONLY = 1;
    public function box(): Box {}
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///room.php", source)
	r, file := NewResolver(idx), parser.ParseFile(source, token.Latest)
	room := ResolvedType{FQN: `App\Room`}
	if m, owner := r.FindMemberOn(room, "ONLY", ConstantAccess); m != nil {
		t.Errorf("Room::ONLY names a constant of Room, and a mixin can't provide one: got %+v on %s", m, owner.FQN)
	}
	if got := r.exprType(`Room::ONLY`, ExprContext{File: file}); !got.IsEmpty() {
		t.Errorf("Room::ONLY isn't typed from the mixin's constant, got %q", got.FullString())
	}
	if m, owner := r.FindMemberOn(room, "box", CallAccess); m == nil || owner.FQN != `App\Extra` {
		t.Errorf("a call still reaches the mixin's method: got %+v on %s", m, owner.FQN)
	}
}
