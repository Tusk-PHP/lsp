package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestEloquentCollectionsAreBuiltByTheModelsNewCollection(t *testing.T) {
	idx := symbols.NewIndex()
	idx.SetCollectionFactories(CollectionFactories())
	idx.IndexFile("file:///vendor/Collection.php", "<?php\nnamespace Illuminate\\Database\\Eloquent;\n/**\n * @template TKey\n * @template TModel\n */\nclass Collection {}\n")
	src := `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Collection;
/** @extends Collection<int, Post> */
class PostCollection extends Collection {}
class Post { public function newCollection(array $models = []): PostCollection {} }
class Shelf {
    /** @return Collection<int, Post> */
    public function posts() {}
}
`
	idx.IndexFile("file:///app/Models/Post.php", src)
	file := parser.ParseFile(src, token.Latest)
	got := resolve.NewResolver(idx).ExpressionType("(new Shelf())->posts()", resolve.ExprContext{File: file}).FQN
	if got != `App\Models\PostCollection` {
		t.Errorf("a model's collection = %q, want the class its newCollection() builds", got)
	}
}
