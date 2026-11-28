package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestTemplateFallbacksTypeLaravelBeforeItsGenerics(t *testing.T) {
	src := `<?php
namespace App;
use Illuminate\Database\Eloquent\Builder;
class User {}
class Query {
    /** @param Builder<User> $q */
    public function run(Builder $q): void {
        echo $q;
    }
}
`
	builder := `<?php
namespace Illuminate\Database\Eloquent;
class Builder {
    public function first() {}
    public function where() {}
    public function get() {}
}
class Collection {}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///builder.php", builder)
	idx.IndexFile("file:///app.php", src)
	idx.SetTemplateFallbacks(TemplateFallbacks())
	r := resolve.NewResolver(idx)
	ctx := resolve.ExprContext{Lines: resolve.SplitLines(src), Pos: textpos.Position{Line: 7, Character: 8}, File: parser.ParseFile(src, token.Latest)}
	for expr, want := range map[string]string{
		"$q->first()":          `?App\User`,
		"$q->where()->first()": `?App\User`,
		"$q->get()":            `Illuminate\Database\Eloquent\Collection<int, App\User>`,
	} {
		if got := r.ExpressionType(expr, ctx).FullString(); got != want {
			t.Errorf("ExpressionType(%q) = %q, want %q", expr, got, want)
		}
	}
}
