package laravel

import (
	"strings"
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
        $first = $q->first();
        $chained = $q->where()->first();
        $all = $q->get();
        echo $first, $chained, $all;
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
	file := parser.ParseFile(src, token.Latest)
	lines := resolve.SplitLines(src)
	line := strings.Count(src[:strings.Index(src, "echo $first")], "\n")
	for v, want := range map[string]string{
		"$first":   `?App\User`,
		"$chained": `?App\User`,
		"$all":     `Illuminate\Database\Eloquent\Collection<int, App\User>`,
	} {
		pos := textpos.Position{Line: line, Character: strings.Index(lines[line], v) + 1}
		if got := r.ResolveVariableTypeTyped(v, lines, pos, file).FullString(); got != want {
			t.Errorf("%s = %q, want %q", v, got, want)
		}
	}
}
