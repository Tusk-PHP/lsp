package completion

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func docMemberTypesProvider(t *testing.T) *Provider {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///vendor/Collection.php", `<?php
namespace Illuminate\Support;

/**
 * @template TKey
 * @template TValue
 */
class Collection {
    /** @return TValue */
    public function first() {}
}
`)
	idx.IndexFile("file:///app/Support/Author.php", "<?php\nnamespace App\\Support;\n\nclass Author { public string $name; }\n")
	idx.IndexFile("file:///app/Models/Post.php", "<?php\nnamespace App\\Models;\n\nclass Post { public string $title; }\n")
	idx.IndexIDEHelperFile("file:///_ide_helper_models.php", `<?php
namespace App\Models;

use Illuminate\Support\Collection;
use App\Support\Author;

/**
 * @property array{id: int, author: Author} $meta
 * @method static Collection<int, Post> all()
 */
class Post {}
`)
	return NewProvider(idx, nil, "")
}

func TestCompletionThroughDocMemberTypes(t *testing.T) {
	p := docMemberTypesProvider(t)
	for _, tt := range []struct{ name, src, want string }{
		{"generic return type", "<?php\nnamespace App\\Http;\n\nuse App\\Models\\Post;\n\nPost::all()->first()->", "title"},
		{"shape property", "<?php\nnamespace App\\Http;\n\nuse App\\Models\\Post;\n\nfunction run(Post $p): void {\n    $p->meta['author']->", "name"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.src, "\n")
			last := len(lines) - 1
			items := p.GetCompletions("file:///app/Http/run.php", tt.src, protocol.Position{Line: last, Character: len(lines[last])})
			if !slices.ContainsFunc(items, func(item protocol.CompletionItem) bool { return item.Label == tt.want }) {
				t.Errorf("completions = %+v, want %q", items, tt.want)
			}
		})
	}
}

func TestTypingThroughDocMemberTypes(t *testing.T) {
	p := docMemberTypesProvider(t)
	src := "<?php\nnamespace App\\Http;\n\nuse App\\Models\\Post;\n\nfunction run(Post $p): void {\n    $posts = Post::all();\n    $author = $p->meta['author'];\n    $id = $p->meta['id'];\n}\n"
	file := parser.ParseFile(src)
	for _, tt := range []struct{ expr, want string }{
		{"$posts", `Illuminate\Support\Collection<int, App\Models\Post>`},
		{"$author", `App\Support\Author`},
		{"$id", "int"},
	} {
		if got := p.ResolveExpressionTypeTyped(tt.expr, src, protocol.Position{Line: 9}, file).String(); got != tt.want {
			t.Errorf("type of %s = %q, want %q", tt.expr, got, tt.want)
		}
	}
}
