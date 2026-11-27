package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

const expressionFormsSource = `<?php
namespace App;
const LIMIT = 3;
function helper(): User { return new User(); }
class User {
    public string $name = '';
    public static ?User $current = null;
    public function fluent(): static { return $this; }
    public function name(): string { return ''; }
    public static function make(): static { return new static(); }
}
class Admin extends User {}
class Query {
    public function where(): static { return $this; }
}
/** @mixin Query */
class Post {}
/** @template TModel */
class Builder {}
class Category {
    public function scopeActive($query) {}
}
class Loose {
    /**
     * @param list<User> $users
     * @param Builder<Category> $categories
     */
    public function run(User $user, Admin $admin, array $list, $any, array $users, Post $post, Builder $categories, string $label): void {
        echo $user; // here
    }
}
`

func expressionFormsFixture(t *testing.T) (*Resolver, ExprContext) {
	t.Helper()
	return fixtureAt(t, expressionFormsSource, "here")
}

// fixtureAt indexes src and returns a resolver with the context of the line
// that ends with `// marker`.
func fixtureAt(t *testing.T, src, marker string) (*Resolver, ExprContext) {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///fixture.php", src)
	lines := SplitLines(src)
	return NewResolver(idx), ExprContext{Lines: lines, Pos: textpos.Position{Line: markedLine(t, lines, marker), Character: 8}, File: parser.ParseFile(src, token.Latest)}
}

func TestExpressionForms(t *testing.T) {
	r, ctx := expressionFormsFixture(t)
	r.Index.SetScopeConventions(map[string]symbols.ScopeConvention{`App\Builder`: {Prefix: "scope"}})
	for _, c := range []struct{ expr, want string }{
		{`"name: $user->name"`, "string"},
		{`<<<TXT
    hi
    TXT`, "string"},
		{"__LINE__", "int"},
		{"__FILE__", "string"},
		{"-1", "int"},
		{"+1.5", "float"},
		{"-'1'", "int|float"},
		{"(int) $any", "int"},
		{"(float) $any", "float"},
		{"(string) $any", "string"},
		{"(bool) $any", "bool"},
		{"(array) $any", "array"},
		{"(object) $any", "object"},
		{"clone($user, ['name' => 'x'])", `App\User`},
		{"$any ?? $user", ""},
		{"$user ?? $admin", `App\User|App\Admin`},
		{"$user |> helper(...)", `App\User`},
		{"$user + 1", ""},
		{"parent::class", "class-string"},
		{"$user::class", "class-string"},
		{"User::class", `class-string<App\User>`},
		{"LIMIT", ""},
		{"self", `App\Loose`},
		{"new $any()", ""},
		{"new (User::class)()", ""},
		{"$admin->fluent()", `App\Admin`},
		{"$user::make()", `App\User`},
		{"$user->$any", ""},
		{"$user->{'name'}", ""},
		{"User::$current", `?App\User`},
		{"$any->name", ""},
		{"[1, 2]", "array<int, int>"},
		{"[]", "array"},
		{"[$user, $any]", "array"},
		{"[...$list]", "array"},
		{"[...$user]", "array"},
		{"['a' => $user, 'b' => 1]", `array{a: App\User, b: int}`},
		{"['a' => $any]", `array{a: mixed}`},
		{"[[1], [2]]", "array<int, array>"},
		{"[['a' => 1], ['a' => 2]]", "array<int, array{a: int}>"},
		{"['a' => \"x$any\", 'b' => true, 'c' => [1]]", "array{a: string, b: bool, c: array<int, int>}"},
		{"[1 => 'a']", "array{1: string}"},
		{"['a' => 1, 2]", "array{a: int}"},
		{"[...$users]", `array<int, App\User>`},
		{"(void) $any", ""},
		{"new $label()", ""},
		{"[1, , 2]", "array<int, int>"},
		{"[, ]", "array"},
		{"['a' => 1, , ]", "array{a: int}"},
		{"['a' => -1]", "array{a: int}"},
		{"parent", ""},
		{"new $user->name()", ""},
		{"User::class::make()", `App\User`},
		{"$post->where()", `App\Post`},
		{"$categories->active()", `App\Builder<App\Category>`},
	} {
		if got := r.ExpressionType(c.expr, ctx).FullString(); got != c.want {
			t.Errorf("ExpressionType(%q) = %q, want %q", c.expr, got, c.want)
		}
	}
}

func TestListValuesOfDifferentTypesAreTheirUnion(t *testing.T) {
	r, ctx := expressionFormsFixture(t)
	for _, c := range []struct{ expr, want string }{
		{"[$user, , $admin]", `App\User|App\Admin`},
		{"[1, 'a']", "int|string"},
		{"[$user, $admin]", `App\User|App\Admin`},
		{"[(int) $any, User::class, -1]", `int|class-string<App\User>`},
	} {
		rt := r.ExpressionType(c.expr, ctx)
		if len(rt.Params) != 2 || rt.Params[1].FullString() != c.want {
			t.Errorf("values of %q = %+v, want %s", c.expr, rt.Params, c.want)
		}
	}
}

func TestExpressionTypeOfWhatIsNoExpression(t *testing.T) {
	r, ctx := expressionFormsFixture(t)
	for _, expr := range []string{"", "/* nothing */", "return 1", "class A {}"} {
		if got := r.ExpressionType(expr, ctx); !got.IsEmpty() {
			t.Errorf("ExpressionType(%q) = %q, want nothing", expr, got.FullString())
		}
	}
	if site := r.CallSiteFor(") + (1", ctx); len(site.Args) != 0 {
		t.Errorf("CallSiteFor of unbalanced arguments = %+v, want no arguments", site.Args)
	}
}

func TestClassNamesNeedAnIndex(t *testing.T) {
	if got := NewResolver(nil).ExpressionType("User", ExprContext{}); !got.IsEmpty() {
		t.Errorf("a class name without an index = %q, want nothing", got.FullString())
	}
}

func TestContainerCallsInAChain(t *testing.T) {
	r, ctx := expressionFormsFixture(t)
	r.ContainerCall = func(_ *Resolver, call, _ string, _ *parser.FileNode) string {
		switch call {
		case "$any->make('user')":
			return `App\User`
		case "$any->make('none')":
			return "-"
		}
		return ""
	}
	for expr, want := range map[string]string{
		"$any->make('user')->name()": "string",
		"$any->make('none')->name()": "",
	} {
		if got := r.ExpressionType(expr, ctx).FullString(); got != want {
			t.Errorf("ExpressionType(%q) = %q, want %q", expr, got, want)
		}
	}
}
