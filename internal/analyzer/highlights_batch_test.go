package analyzer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func highlightsOneByOne(a *Analyzer, uri, source string, pos protocol.Position) []protocol.DocumentHighlight {
	r := a.engine.Resolver()
	doc := sourcectx.IndexedDocument(a.index, uri, source)
	ctx := doc.NameAt(pos)
	if ctx == nil || ctx.SymbolText == "" {
		return nil
	}
	if a.mayBeLocalVariable(ctx) {
		if highlights := highlightVariableOccurrences(source, pos, ctx.SymbolText); highlights != nil {
			return highlights
		}
	}
	target := a.resolveSymbolAt(r, ctx, source)
	if target == nil {
		return nil
	}
	var decl protocol.Range
	if target.URI == uri {
		decl = target.Range
	}
	tokens := doc.Significant()
	var interpolated interpolatedStrings
	var highlights []protocol.DocumentHighlight
	for i, tok := range tokens {
		if interpolated.skip(tok.Kind) {
			continue
		}
		name, ok := mayName(tokens, i, source, target)
		if !ok {
			continue
		}
		rng := doc.Range(name.Start, name.End)
		if tok.Kind == token.Variable && !follows(tokens, i, token.DoubleColon) {
			if !sameRange(rng, decl) {
				continue
			}
		} else if !sameSymbol(a.resolveSymbolAt(r, doc.NameAt(rng.Start), source), target) {
			continue
		}
		highlights = append(highlights, protocol.DocumentHighlight{Range: rng, Kind: highlightKind(rng, decl, pos)})
	}
	return highlights
}

const batchHighlightRooms = `<?php
namespace App;

use App\Models\User;

class User2 { public string $name = ''; public static int $count = 0; public function rename(string $to): static { return $this; } }

class Room {
    private User2 $owner;
    public function __construct(User2 $owner) { $this->owner = $owner; }
    public function area(User2 $user): int {
        $user->rename('a')->rename('b');
        $other = $user;
        $other = new User2();
        $fn = fn (User2 $u) => $u->rename($user->name);
        User2::$count++;
        return $this->owner->rename("x {$user->name}")->name === '' ? 1 : strlen($other->name);
    }
    public function size(): int { return $this->area($this->owner) + User2::$count; }
}

function helper(Room $room, User2 $user): int {
    $closure = function () use ($user) { return $user->rename('c'); };
    return $room->area($user) + $room->size();
}
`

func TestHighlightsMatchResolvingEachOccurrenceAlone(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/rooms.php", batchHighlightRooms)
	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))
	answered := 0
	for line, text := range strings.Split(batchHighlightRooms, "\n") {
		for col := 0; col <= len(text); col++ {
			pos := protocol.Position{Line: line, Character: col}
			got, want := a.GetDocumentHighlights("file:///app/rooms.php", batchHighlightRooms, pos), highlightsOneByOne(a, "file:///app/rooms.php", batchHighlightRooms, pos)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("at %v: %+v, one by one %+v", pos, got, want)
			}
			if len(got) > 1 {
				answered++
			}
		}
	}
	if answered < 100 {
		t.Errorf("only %d positions highlight more than one occurrence", answered)
	}
}
