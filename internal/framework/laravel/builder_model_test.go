package laravel

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestBuilderModel(t *testing.T) {
	idx := queriedModelIndex()
	src := "<?php\nnamespace App\\Http;\nuse App\\Models\\Post;\nuse App\\Models\\Plain;\n/** @param \\Illuminate\\Database\\Eloquent\\Builder<Post> $query */\nfunction run($query) {\n    return 1;\n}\n"
	idx.IndexFile("file:///app/Http/run.php", src)
	file := parser.ParseFile(src, token.Latest)
	line := strings.Count(src[:strings.Index(src, "return 1;")], "\n")
	r := resolve.NewResolver(idx)
	for _, c := range []struct{ prefix, method, want string }{
		{"Post::where(", "where", `App\Models\Post`},
		{"\\App\\Models\\Post::where(", "where", `App\Models\Post`},
		{"$query->where(", "where", `App\Models\Post`},
		{"Plain::where(", "where", ""},
		{"Post::where(", "with", ""},
		{"::where(", "where", ""},
		{"$unknown->where(", "where", ""},
	} {
		pos := textpos.Position{Line: line, Character: len(c.prefix)}
		if got := BuilderModel(r, idx, c.prefix, c.method, src, pos, file).FQN; got != c.want {
			t.Errorf("BuilderModel(%q, %q) = %q, want %q", c.prefix, c.method, got, c.want)
		}
	}
}
