package completion

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// A model with both kinds of local scope: the scope prefix, and Laravel 12's
// #[Scope] attribute on a protected method.
const postModelSource = `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Attributes\Scope;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Database\Eloquent\Model;

class Post extends Model
{
    /** Only posts that are out. */
    public function scopePublished(Builder $query): void
    {
        $query->whereNotNull('published_at');
    }

    public function scopeOfType(Builder $query, string $type): void
    {
        $query->where('type', $type);
    }

    #[Scope]
    protected function popular(Builder $query): void
    {
        $query->where('votes', '>', 100);
    }

    public function title(): string
    {
        return '';
    }
}
`

func TestEloquentLocalScopes(t *testing.T) {
	p := setupLaravelE2E(t)
	p.index.IndexFile("file:///app/Models/Post.php", postModelSource)

	preamble := "<?php\nuse App\\Models\\Post;\n\n\n\n"
	file := parser.ParseFile(preamble, token.Latest)
	typed := func(expr string) string {
		return p.exprTypeAt(expr, preamble, protocol.Position{Line: 4}, file).String()
	}
	for expr, want := range map[string]string{
		"Post::published()":                       "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\Post>",
		"Post::ofType('news')":                    "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\Post>",
		"Post::popular()":                         "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\Post>",
		"Post::query()->published()":              "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\Post>",
		"Post::where('a', 1)->popular()->first()": "?App\\Models\\Post",
		"Post::published()->get()":                "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Post>",
	} {
		if got := typed(expr); got != want {
			t.Errorf("%s: got %q, want %q", expr, got, want)
		}
	}

	labels := func(src string) map[string]bool {
		items := p.GetCompletions("file:///test.php", src, protocol.Position{Line: strings.Count(src, "\n"), Character: len(src) - strings.LastIndex(src, "\n") - 1})
		out := map[string]bool{}
		for _, it := range items {
			out[it.Label] = true
		}
		return out
	}
	static := labels("<?php\nuse App\\Models\\Post;\nPost::")
	for _, want := range []string{"published", "ofType", "popular", "where"} {
		if !static[want] {
			t.Errorf("Post:: is missing %s", want)
		}
	}
	if static["scopePublished"] {
		t.Errorf("Post:: lists the scopePublished method itself")
	}
	builder := labels("<?php\nuse App\\Models\\Post;\nPost::query()->")
	for _, want := range []string{"published", "ofType", "popular", "where"} {
		if !builder[want] {
			t.Errorf("Post::query()-> is missing %s", want)
		}
	}
}
