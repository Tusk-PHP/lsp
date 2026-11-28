package laravel

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
)

func TestBuilderModel(t *testing.T) {
	for _, c := range []struct{ call, receiver, want string }{
		{"Post::where('a');", "Post", `App\Models\Post`},
		{"\\App\\Models\\Post::where('a');", "\\App\\Models\\Post", `App\Models\Post`},
		{"$query->where('a');", "$query", `App\Models\Post`},
		{"$query\n        // ->with(\n        ->where('a');", "$query", `App\Models\Post`},
		{"Plain::where('a');", "Plain", ""},
		{"$unknown->where('a');", "$unknown", ""},
	} {
		idx := queriedModelIndex()
		src := "<?php\nnamespace App\\Http;\nuse App\\Models\\Post;\nuse App\\Models\\Plain;\n/** @param \\Illuminate\\Database\\Eloquent\\Builder<Post> $query */\nfunction run($query) {\n    return " + c.call + "\n}\n"
		idx.IndexFile("file:///app/Http/run.php", src)
		file := parser.ParseFile(src, token.Latest)
		at := uint32(strings.Index(src, c.call))
		receiver := ast.ExprSpanning(file.Tree, ast.Span{Start: at, End: at + uint32(len(c.receiver))})
		if receiver == nil {
			t.Fatalf("no node %q in %q", c.receiver, src)
		}
		if got := BuilderModel(resolve.NewResolver(idx), idx, file, receiver).FQN; got != c.want {
			t.Errorf("BuilderModel(%q) = %q, want %q", c.call, got, c.want)
		}
	}
	if got := BuilderModel(resolve.NewResolver(queriedModelIndex()), queriedModelIndex(), nil, nil).FQN; got != "" {
		t.Errorf("BuilderModel without a file = %q, want none", got)
	}
}
