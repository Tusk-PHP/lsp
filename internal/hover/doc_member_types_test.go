package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverDocMemberTypesKeepTheirShape(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Models/Post.php", "<?php\nnamespace App\\Models;\n\nclass Post {}\n")
	idx.IndexIDEHelperFile("file:///_ide_helper_models.php", `<?php
namespace App\Models;

use Illuminate\Support\Collection;

/**
 * @property callable(int): bool $handler
 * @method static Collection<int, Post> all()
 */
class Post {}
`)
	src := "<?php\nnamespace App\\Http;\n\nuse App\\Models\\Post;\n\nfunction run(Post $p): void {\n    $p->handler;\n    Post::all();\n}\n"
	p := NewProvider(idx, nil, "none")
	for _, tt := range []struct{ line, word, want string }{
		{"$p->handler", "handler", "(magic) public callable(int): bool $handler"},
		{"Post::all", "all", "(magic) public static function all(): Illuminate\\Support\\Collection<int, App\\Models\\Post>"},
	} {
		if got := hoverAt(t, p, src, tt.line, tt.word); !strings.Contains(got, tt.want) {
			t.Errorf("hover on %s = %q, want it to contain %q", tt.word, got, tt.want)
		}
	}
}
