package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const fetchTypesLibrary = `<?php
namespace App\Support;
/** @template TModel */
class Builder { public function where($column): static { return $this; } }
class Post {}
class Holder {
    /** @var Builder<Post> */
    public static $shared;
    public static Builder $native;
    /** @var array<string, Builder<Post>> */
    public array $builders;
    /** @var Builder<Post>[] */
    public array $legacy;
    /** @var list<Builder<Post>> */
    public array $list;
    /** @var array{posts: Builder<Post>, count: int} */
    public array $shape;
    public static function own(): void {
        self::$shared->where('a');
        static::$shared->where('a');
    }
}
/** @return Builder<Post> */
function postQuery() {}
`

const fetchTypesCaller = `<?php
namespace App\Http;
use App\Support\Builder;
use App\Support\Holder;
use function App\Support\postQuery;
use function App\Support\postQuery as posts;
function run(Holder $holder): void {
    /** @var array<int, Builder<\App\Support\Post>> $local */
    $local = load();
    Holder::$shared->where('a');
    Holder::$native->where('a');
    $holder->builders['x']->where('a');
    $holder->legacy[0]->where('a');
    $holder->list[0]->where('a');
    $holder->shape['posts']->where('a');
    $holder->shape['count'];
    $local[0]->where('a');
    postQuery()->where('a');
    posts()->where('a');
    \App\Support\postQuery()->where('a');
}
`

func fetchTypesResolver() *Resolver {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///lib.php", fetchTypesLibrary)
	idx.IndexFile("file:///run.php", fetchTypesCaller)
	return NewResolver(idx)
}

func nodeTypeOf(t *testing.T, r *Resolver, file *parser.FileNode, expr string) string {
	t.Helper()
	start := strings.Index(file.Source, expr)
	if start < 0 {
		t.Fatalf("no %q in the source", expr)
	}
	e := ast.ExprSpanning(file.Tree, ast.Span{Start: uint32(start), End: uint32(start + len(expr))})
	if e == nil {
		t.Fatalf("no node spans %q", expr)
	}
	return r.NodeType(file, e).FullString()
}

const postBuilder = `App\Support\Builder<App\Support\Post>`

func TestStaticPropertyFetchesTakeTheDeclaredType(t *testing.T) {
	r := fetchTypesResolver()
	caller := parser.ParseFile(fetchTypesCaller, token.Latest)
	for expr, want := range map[string]string{
		"Holder::$shared":             postBuilder,
		"Holder::$native":             `App\Support\Builder`,
		"Holder::$shared->where('a')": postBuilder,
		"Holder::$native->where('a')": `App\Support\Builder`,
	} {
		if got := nodeTypeOf(t, r, caller, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
	library := parser.ParseFile(fetchTypesLibrary, token.Latest)
	for _, expr := range []string{"self::$shared", "static::$shared", "self::$shared->where('a')"} {
		if got := nodeTypeOf(t, r, library, expr); got != postBuilder {
			t.Errorf("%s = %q, want %q", expr, got, postBuilder)
		}
	}
}

func TestArrayElementFetchesTakeTheValueType(t *testing.T) {
	r := fetchTypesResolver()
	caller := parser.ParseFile(fetchTypesCaller, token.Latest)
	for expr, want := range map[string]string{
		"$holder->builders['x']":              postBuilder,
		"$holder->builders['x']->where('a')":  postBuilder,
		"$holder->legacy[0]":                  postBuilder,
		"$holder->list[0]":                    postBuilder,
		"$holder->shape['posts']":             postBuilder,
		"$holder->shape['posts']->where('a')": postBuilder,
		"$holder->shape['count']":             "int",
		"$local[0]":                           postBuilder,
		"$local[0]->where('a')":               postBuilder,
	} {
		if got := nodeTypeOf(t, r, caller, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

func TestCallsThroughUseFunctionTakeTheFunctionsReturnType(t *testing.T) {
	r := fetchTypesResolver()
	caller := parser.ParseFile(fetchTypesCaller, token.Latest)
	for _, expr := range []string{
		"postQuery()",
		"postQuery()->where('a')",
		"posts()",
		"posts()->where('a')",
		`\App\Support\postQuery()->where('a')`,
	} {
		if got := nodeTypeOf(t, r, caller, expr); got != postBuilder {
			t.Errorf("%s = %q, want %q", expr, got, postBuilder)
		}
	}
}
