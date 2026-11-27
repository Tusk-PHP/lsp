package sourcectx

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const indexedRoom = `<?php
namespace App;

use App\Models\User;
use function App\Support\format;

#[Entity(table: 'rooms')]
class Room extends Base {
    public function __construct(private User $owner, int $size = 0) {}
    public function area(int $width): int {
        $fn = fn ($x) => $x * $width;
        return format($this->owner->name(), $fn(2), new static($this->owner));
    }
}

namespace App\Other;

function helper() {
    return new class(1) extends \App\Room {
        public function area(int $width): int { return parent::area($width); }
    };
}
`

func positionsOf(src string) []protocol.Position {
	var out []protocol.Position
	for line, text := range strings.Split(src, "\n") {
		for col := 0; col <= len(text); col++ {
			out = append(out, protocol.Position{Line: line, Character: col})
		}
	}
	return out
}

func TestIndexedDocumentAnswersAsAFreshParse(t *testing.T) {
	edited := strings.Replace(indexedRoom, "        $fn =", "        $extra = 1;\n        $fn =", 1)
	cases := []struct {
		name    string
		indexed []string
		source  string
		reused  bool
	}{
		{"the text just indexed", []string{indexedRoom}, indexedRoom, true},
		{"an edit the index hasn't read", []string{indexedRoom}, edited, false},
		{"an edit indexed again", []string{indexedRoom, edited}, edited, true},
		{"a file the index doesn't have", nil, indexedRoom, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := symbols.NewIndex()
			for _, text := range tc.indexed {
				idx.IndexFile("file:///room.php", text)
			}
			if _, _, ok := idx.IndexedParse("file:///room.php", tc.source); ok != tc.reused {
				t.Fatalf("indexed parse reused = %v, want %v", ok, tc.reused)
			}
			indexed := IndexedDocument(idx, "file:///room.php", tc.source)
			fresh := NewDocument("file:///room.php", tc.source, idx.GrammarOf("file:///room.php"))
			if !reflect.DeepEqual(indexed.File(), fresh.File()) {
				t.Fatal("the indexed document's declarations differ from a fresh parse")
			}
			for _, pos := range positionsOf(tc.source) {
				if got, want := indexed.At(pos), fresh.At(pos); !reflect.DeepEqual(got, want) {
					t.Fatalf("at %v: %+v, a fresh parse gives %+v", pos, got, want)
				}
			}
		})
	}
}
