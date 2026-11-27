package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestMemberCompletionThroughTheElementsOfAMagicProperty(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///app/Post.php", `<?php
namespace App;
class Tag { public function label(): string { return ''; } }
/**
 * @property list<Tag> $tags
 * @property array<string, Tag> $byName
 * @property non-empty-array<int, Tag> $first
 */
class Post {}
`)
	p := NewProvider(idx, nil, "none")
	for _, receiver := range []string{"$p->tags[0]", "$p->byName['x']", "$p->first[0]"} {
		if !completeMarked(p, "<?php\nnamespace Other;\nuse App\\Post;\nfunction f(Post $p) {\n    "+receiver+"->‸\n}\n")["label"] {
			t.Errorf("%s-> offers Tag's label()", receiver)
		}
	}
}
