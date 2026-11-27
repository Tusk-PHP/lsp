package completion

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const shapeReceivers = `<?php
namespace App;
/** @property array{id: int, tags: list<string>, author: array{name: string, email?: string}} $meta */
class Post {
    /** @var array{title: string, slug: string} */
    public array $info;
    /** @var array{hits: int, misses: int} */
    public static array $stats;
    /** @return array{id: int, total: float} */
    public function summary(): array { return []; }
    public function self(): void {
        echo self::$stats[''];
    }
}
/** @return array{code: int, reason: string} */
function status(): array { return []; }
`

func TestArrayKeysOnAShapeTypedReceiver(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///app/Post.php", shapeReceivers)
	p := NewProvider(idx, nil, "none")
	prefix := "<?php\nnamespace App;\nfunction f(Post $p, ?Post $maybe) {\n    "
	for _, tc := range []struct {
		code string
		want []string
	}{
		{"$p->meta['‸", []string{"author", "id", "tags"}},
		{"$p->meta['‸']", []string{"author", "id", "tags"}},
		{"$p->meta['t‸", []string{"tags"}},
		{"$p->meta[‸", []string{"author", "id", "tags"}},
		{"$maybe?->meta['‸", []string{"author", "id", "tags"}},
		{"$p->info['‸", []string{"slug", "title"}},
		{"Post::$stats['‸", []string{"hits", "misses"}},
		{"$p->summary()['‸", []string{"id", "total"}},
		{"status()['‸", []string{"code", "reason"}},
		{"$p->meta['author']['‸", []string{"email", "name"}},
		{"$p->meta['tags']['‸", nil},
		{"$p->meta['missing']['‸", nil},
		{"$p->unknown['‸", nil},
		{"$p->meta[$k]['‸", nil},
	} {
		got := mapKeys(completeMarked(p, prefix+tc.code+"\n}\n"), 10)
		slices.Sort(got)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.code, got, tc.want)
		}
	}

	items := completeMarkedItems(p, prefix+"$p->meta['author']['‸\n}\n")
	if email, ok := itemLabeled(items, "email"); !ok || email.Detail != "string (optional)" || email.SortText != "1email" {
		t.Errorf("an optional key is marked optional and sorted last, got %+v", email)
	}
	if name, ok := itemLabeled(items, "name"); !ok || name.InsertText != "name" {
		t.Errorf("a key inside quotes inserts the bare key, got %+v", name)
	}
	if tags, ok := itemLabeled(completeMarkedItems(p, prefix+"$p->meta[‸\n}\n"), "tags"); !ok || tags.InsertText != "'tags'" {
		t.Errorf("a key without quotes inserts a quoted key, got %+v", tags)
	}

	lib := idx.GetFileSource("file:///app/Post.php")
	at := strings.Index(lib, "self::$stats['") + len("self::$stats['")
	marked := lib[:at] + "‸" + lib[at:]
	if got := mapKeys(completeMarked(p, marked), 10); !slices.Contains(got, "hits") {
		t.Errorf("self::$stats[' offers its keys, got %v", got)
	}
}
