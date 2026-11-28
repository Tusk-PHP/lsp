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
	for _, c := range []struct{ call, method, want string }{
		{"Post::where('a');", "where", `App\Models\Post`},
		{"\\App\\Models\\Post::where('a');", "where", `App\Models\Post`},
		{"$query->where('a');", "where", `App\Models\Post`},
		{"$query\n        // ->with(\n        ->where('a');", "where", `App\Models\Post`},
		{"Plain::where('a');", "where", ""},
		{"Post::where('a');", "with", ""},
		{"// Post::where('a');", "where", ""},
		{"$unknown->where('a');", "where", ""},
	} {
		idx := queriedModelIndex()
		src := "<?php\nnamespace App\\Http;\nuse App\\Models\\Post;\nuse App\\Models\\Plain;\n/** @param \\Illuminate\\Database\\Eloquent\\Builder<Post> $query */\nfunction run($query) {\n    return " + c.call + "\n}\n"
		idx.IndexFile("file:///app/Http/run.php", src)
		file := parser.ParseFile(src, token.Latest)
		at := strings.LastIndex(src, "where(") + len("where(")
		line := strings.Count(src[:at], "\n")
		pos := textpos.Position{Line: line, Character: at - strings.LastIndex(src[:at], "\n") - 1}
		if got := BuilderModel(resolve.NewResolver(idx), idx, c.method, src, pos, file).FQN; got != c.want {
			t.Errorf("BuilderModel(%q, %q) = %q, want %q", c.call, c.method, got, c.want)
		}
	}
	if got := BuilderModel(resolve.NewResolver(queriedModelIndex()), queriedModelIndex(), "where", "<?php\nPost::where('a');", textpos.Position{Line: 1, Character: 12}, nil).FQN; got != "" {
		t.Errorf("BuilderModel without a file = %q, want none", got)
	}
}
