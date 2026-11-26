package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestResolveClassNameReturnsTheDeclaredCase(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///post.php", "<?php\nnamespace App\\Models;\nclass Post {}\n")
	idx.IndexFile("file:///tag.php", "<?php\nnamespace App\\Models\\Post;\nclass Tag {}\n")
	r := NewResolver(idx)
	file := parser.ParseFile("<?php\nnamespace App\\Models;\nuse App\\Models as Db;\nuse App\\Models\\Post as Article;\n")
	for name, want := range map[string]string{
		`\APP\MODELS\POST`: `App\Models\Post`,
		"article":          `App\Models\Post`,
		`DB\POST`:          `App\Models\Post`,
		`db\post\TAG`:      `App\Models\Post\Tag`,
		"POST":             `App\Models\Post`,
		`DB\Missing`:       `App\Models\Missing`,
		`\Nowhere\Thing`:   `Nowhere\Thing`,
	} {
		if got := r.ResolveClassName(name, file); got != want {
			t.Errorf("ResolveClassName(%q) = %q, want %q", name, got, want)
		}
	}
}
