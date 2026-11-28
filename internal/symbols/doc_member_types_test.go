package symbols

import "testing"

const docMemberTypesModel = `<?php
namespace App\Models;

use Illuminate\Support\Collection;
use App\Support\Author;

/**
 * @template TKey
 * @property array{id: int, author: Author, tags: list<string>} $meta
 * @property TKey $key
 * @property Author|null $owner
 * @property-read Comment[] $comments
 * @method static Collection<int, Post> all()
 * @method Closure(int): Author maker()
 * @method callable(int, string): bool checker()
 * @method (TKey is int ? Author : null) pick()
 * @method int | null count()
 * @method void fill(Collection<int, Author> $items, \Closure<T>(T): T $map)
 */
class Post {}
`

func TestDocMemberTypesResolvePartByPart(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///post.php", docMemberTypesModel)
	assertDocMemberTypes(t, idx)
}

func TestIDEHelperDocMemberTypesResolvePartByPart(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///post.php", "<?php\nnamespace App\\Models;\n\n/** @template TKey */\nclass Post {}\n")
	idx.IndexIDEHelperFile("file:///_ide_helper_models.php", docMemberTypesModel)
	assertDocMemberTypes(t, idx)
}

func assertDocMemberTypes(t *testing.T, idx *Index) {
	t.Helper()
	tests := []struct {
		member string
		want   string
		param  int
	}{
		{"$meta", `array{id: int, author: App\Support\Author, tags: list<string>}`, -1},
		{"$key", "TKey", -1},
		{"$owner", `App\Support\Author|null`, -1},
		{"$comments", `App\Models\Comment[]`, -1},
		{"all", `Illuminate\Support\Collection<int, App\Models\Post>`, -1},
		{"maker", `Closure(int): App\Support\Author`, -1},
		{"checker", "callable(int, string): bool", -1},
		{"pick", `(TKey is int ? App\Support\Author : null)`, -1},
		{"count", "int | null", -1},
		{"fill", `Illuminate\Support\Collection<int, App\Support\Author>`, 0},
		{"fill", `\Closure<T>(T): T`, 1},
	}
	for _, tt := range tests {
		sym := idx.Lookup(`App\Models\Post::` + tt.member)
		if sym == nil {
			t.Errorf("no member %s", tt.member)
			continue
		}
		got := sym.Type
		switch {
		case tt.param >= 0:
			got = sym.Params[tt.param].Type
		case sym.Kind == KindMethod:
			got = sym.ReturnType
		}
		if got != tt.want {
			t.Errorf("%s type = %q, want %q", tt.member, got, tt.want)
		}
	}
}
