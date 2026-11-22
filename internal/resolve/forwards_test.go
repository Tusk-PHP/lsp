package resolve

import (
	"sort"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A Laravel-shaped model and query builders: the model forwards calls it
// doesn't have to Builder<static>, which brings in the base query builder
// through @mixin, and Builder finds its model's local scopes.
const forwardsVendor = `<?php
namespace Illuminate\Database\Eloquent;

abstract class Model {
    /** @return \Illuminate\Database\Eloquent\Builder<static> */
    public static function query() {}
    public function save(): bool { return true; }
}

/**
 * @template TModel of Model
 * @mixin \Illuminate\Database\Query\Builder
 */
class Builder {
    /** @return $this */
    public function where($column, $value = null) {}
    /** @return TModel|null */
    public function first() {}
    /** @return \Illuminate\Database\Eloquent\Collection<int, TModel> */
    public function get() {}
    protected function hidden() {}
}

/** @template TModel */
class Collection {}
`

const forwardsQuery = `<?php
namespace Illuminate\Database\Query;

class Builder {
    /** @return $this */
    public function orderBy($column) {}
    public function toSql(): string { return ''; }
}
`

const forwardsModels = `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Attributes\Scope;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Database\Eloquent\Model;

class Category extends Model {
    public string $name;

    /** Only active categories. */
    public function scopeActive(Builder $query): void {}

    public function scopeOfType(Builder $query, string $type): void {}

    #[Scope]
    protected function popular(Builder $query): void {}

    public static function scopeNotAScope(): void {}

    public function title(): string { return ''; }
}

class Plain {
    public function run(): void {}
}
`

func setupForwardsResolver(t *testing.T) *Resolver {
	t.Helper()
	idx := symbols.NewIndex()
	idx.SetCallForwards(map[string][]string{
		"Illuminate\\Database\\Eloquent\\Model": {"\\Illuminate\\Database\\Eloquent\\Builder<static>"},
	})
	idx.SetScopeConventions(map[string]symbols.ScopeConvention{
		"Illuminate\\Database\\Eloquent\\Builder": {Prefix: "scope", Attribute: "Illuminate\\Database\\Eloquent\\Attributes\\Scope"},
	})
	idx.IndexFile("file:///vendor/Eloquent.php", forwardsVendor)
	idx.IndexFile("file:///vendor/Query.php", forwardsQuery)
	idx.IndexFile("file:///app/Models.php", forwardsModels)
	return NewResolver(idx)
}

const (
	categoryFQN = "App\\Models\\Category"
	builderFQN  = "Illuminate\\Database\\Eloquent\\Builder"
)

var categoryBuilder = ResolvedType{FQN: builderFQN, Params: []ResolvedType{{FQN: categoryFQN}}}

func TestForwardTypes(t *testing.T) {
	r := setupForwardsResolver(t)
	got := r.ForwardTypes(ResolvedType{FQN: categoryFQN})
	if len(got) != 1 || got[0].String() != builderFQN+"<"+categoryFQN+">" {
		t.Fatalf("Category forwards to %v, want Builder<Category>", got)
	}
	for _, rt := range []ResolvedType{{}, {FQN: "int"}, {FQN: "App\\Models\\Plain"}} {
		if got := r.ForwardTypes(rt); len(got) != 0 {
			t.Errorf("%q forwards to %v, want nothing", rt.FQN, got)
		}
	}
	if got := NewResolver(nil).ForwardTypes(ResolvedType{FQN: categoryFQN}); got != nil {
		t.Errorf("a resolver without an index forwards to %v", got)
	}
}

func TestForwardedReceiver(t *testing.T) {
	r := setupForwardsResolver(t)
	category := ResolvedType{FQN: categoryFQN}
	cases := []struct {
		name      string
		call      bool
		forwarded bool
	}{
		{"where", true, true},     // on the builder
		{"orderBy", true, true},   // on the base query builder, through @mixin
		{"active", true, true},    // a prefixed scope
		{"popular", true, true},   // a #[Scope] method, declared on the model but protected
		{"save", true, false},     // the model's own method
		{"query", true, false},    // the model's own static method
		{"name", false, false},    // the model's own property
		{"nothing", true, false},  // nowhere
		{"popular", false, false}, // not a call: no scope
	}
	for _, c := range cases {
		fwd, ok := r.ForwardedReceiver(category, c.name, c.call)
		if ok != c.forwarded {
			t.Errorf("%s (call %v): forwarded %v, want %v", c.name, c.call, ok, c.forwarded)
			continue
		}
		if ok && fwd.String() != builderFQN+"<"+categoryFQN+">" {
			t.Errorf("%s: forwarded to %s", c.name, fwd.String())
		}
	}
}

func TestLookupMemberAndFindMemberAnywhere(t *testing.T) {
	r := setupForwardsResolver(t)
	category := ResolvedType{FQN: categoryFQN}
	for name, wantParent := range map[string]string{
		"where":   builderFQN,
		"orderBy": "Illuminate\\Database\\Query\\Builder",
		"active":  categoryFQN, // scopeActive
		"popular": categoryFQN,
		"save":    "Illuminate\\Database\\Eloquent\\Model",
	} {
		m, _ := r.LookupMember(category, name, true)
		if m == nil || m.ParentFQN != wantParent {
			t.Errorf("LookupMember(Category, %s) = %v, want a member of %s", name, m, wantParent)
		}
		if m := r.FindMemberAnywhere(categoryFQN, name); m == nil || m.ParentFQN != wantParent {
			t.Errorf("FindMemberAnywhere(Category, %s) = %v", name, m)
		}
	}
	if m, _ := r.LookupMember(category, "nothing", true); m != nil {
		t.Errorf("LookupMember found %v for a missing member", m)
	}
	// A property is found by a plain access.
	if m := r.FindMemberAnywhere(categoryFQN, "$name"); m == nil {
		t.Errorf("FindMemberAnywhere(Category, $name) found nothing")
	}
	if m := r.FindMemberAnywhere(categoryFQN, "nothing"); m != nil {
		t.Errorf("FindMemberAnywhere found %v for a missing member", m)
	}
	if m := r.FindMemberAnywhereOn(ResolvedType{}, "where"); m != nil {
		t.Errorf("FindMemberAnywhereOn found %v on no type", m)
	}
	// On the builder with its model type argument, a scope is found on the
	// model; without the type argument the model isn't known.
	if m := r.FindMemberAnywhereOn(categoryBuilder, "active"); m == nil || m.Name != "scopeActive" {
		t.Errorf("FindMemberAnywhereOn(Builder<Category>, active) = %v", m)
	}
	if m, owner := r.LookupMember(categoryBuilder, "ofType", true); m == nil || owner.FQN != builderFQN {
		t.Errorf("LookupMember(Builder<Category>, ofType) = %v on %v", m, owner)
	}
	if m := r.FindMemberAnywhere(builderFQN, "active"); m != nil {
		t.Errorf("a scope was found on Builder without its model: %v", m)
	}
}

func TestScopeMethodAndScopes(t *testing.T) {
	r := setupForwardsResolver(t)
	for name, want := range map[string]string{"active": "scopeActive", "ofType": "scopeOfType", "popular": "popular"} {
		if m := r.ScopeMethod(categoryBuilder, name); m == nil || m.Name != want {
			t.Errorf("ScopeMethod(%s) = %v, want %s", name, m, want)
		}
	}
	for _, name := range []string{"title", "notAScope", "nothing", ""} {
		if m := r.ScopeMethod(categoryBuilder, name); m != nil {
			t.Errorf("ScopeMethod(%q) = %s, want none", name, m.Name)
		}
	}
	for _, rt := range []ResolvedType{
		{FQN: builderFQN}, // no model
		{FQN: builderFQN, Params: []ResolvedType{{FQN: "int"}}},     // not a class
		{FQN: "App\\Models\\Plain", Params: categoryBuilder.Params}, // no convention
		{FQN: "string", Params: categoryBuilder.Params},
	} {
		if m := r.ScopeMethod(rt, "active"); m != nil {
			t.Errorf("ScopeMethod on %s found %s", rt.String(), m.Name)
		}
		if got := r.Scopes(rt); len(got) != 0 {
			t.Errorf("Scopes on %s = %v", rt.String(), got)
		}
	}
	if m := NewResolver(nil).ScopeMethod(categoryBuilder, "active"); m != nil {
		t.Errorf("a resolver without an index found %v", m)
	}

	var names []string
	for _, s := range r.Scopes(categoryBuilder) {
		names = append(names, s.Name+"="+s.Method.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, " "); got != "active=scopeActive ofType=scopeOfType popular=popular" {
		t.Errorf("Scopes = %s", got)
	}
}

func TestForwardedAndScopedCallTypes(t *testing.T) {
	r := setupForwardsResolver(t)
	src := "<?php\nuse App\\Models\\Category;\n"
	ctx := ExprContext{File: parser.ParseFile(src)}
	for expr, want := range map[string]string{
		"Category::where('a')":                    builderFQN + "<" + categoryFQN + ">",
		"Category::orderBy('a')":                  builderFQN + "<" + categoryFQN + ">",
		"Category::where('a')->first()":           "?" + categoryFQN,
		"Category::active()":                      builderFQN + "<" + categoryFQN + ">",
		"Category::popular()->get()":              "Illuminate\\Database\\Eloquent\\Collection<int, " + categoryFQN + ">",
		"Category::query()->ofType('x')->first()": "?" + categoryFQN,
		"Category::query()->toSql()":              "string",
	} {
		if got := r.ExpressionType(expr, ctx).FullString(); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
	if got := r.ExpressionType("Category::nothing()", ctx); !got.IsEmpty() {
		t.Errorf("Category::nothing() = %q, want nothing", got.FullString())
	}
}
