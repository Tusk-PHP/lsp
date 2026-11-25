package analyzer

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const referencesRoom = `<?php
namespace App;

class Room {
    public float $area = 0.0;

    public function area(): float {
        return $this->area;
    }
}
`

const referencesShape = `<?php
namespace App;

class Shape {
    public float $area = 0.0;

    public function area(): float { return $this->area; }
}
`

const referencesUse = `<?php
namespace App;

function measure(Room $room, Shape $shape): string {
    // area() of the room
    $x = $room->area() + $shape->area() + $room->area + $shape->area;
    return "area: {$room->area()}" . area . $room->area();
}
`

func referencesAt(t *testing.T, uri, needle string) []string {
	t.Helper()
	sources := map[string]string{"file:///Room.php": referencesRoom, "file:///Shape.php": referencesShape, "file:///use.php": referencesUse}
	a, read := setupRenameAnalyzer(sources)
	src := sources[uri]
	var got []string
	for _, l := range a.FindAllReferences(uri, src, positionOf(t, src, needle), read) {
		got = append(got, fmt.Sprintf("%s:%d:%d", strings.TrimPrefix(l.URI, "file:///"), l.Range.Start.Line, l.Range.Start.Character))
	}
	slices.Sort(got)
	return got
}

func TestReferencesOfAMethodAreOnlyItsResolvedUses(t *testing.T) {
	got := referencesAt(t, "file:///use.php", "area() +")
	want := []string{"Room.php:6:20", "use.php:5:16", "use.php:6:51"}
	if !slices.Equal(got, want) {
		t.Errorf("Room::area's declaration and its calls on a Room; never Shape::area, the properties, the comment, the string or the constant:\n got %v\nwant %v", got, want)
	}
}

func TestReferencesOfAPropertyAreOnlyItsDeclarationAndAccesses(t *testing.T) {
	got := referencesAt(t, "file:///Room.php", "$area = 0.0")
	want := []string{"Room.php:4:17", "Room.php:7:22", "use.php:5:49"}
	if !slices.Equal(got, want) {
		t.Errorf("Room::$area's declaration and accesses on a Room, never Shape's:\n got %v\nwant %v", got, want)
	}
}

func TestReferencesOfANameThatResolvesToNothing(t *testing.T) {
	if got := referencesAt(t, "file:///use.php", "area . $room"); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestRenameEditsOnlyTheResolvedUses(t *testing.T) {
	sources := map[string]string{"file:///Room.php": referencesRoom, "file:///Shape.php": referencesShape, "file:///use.php": referencesUse}
	a, read := setupRenameAnalyzer(sources)
	edit := a.Rename("file:///Room.php", referencesRoom, positionOf(t, referencesRoom, "area(): float"), "size", read)
	if edit == nil {
		t.Fatal("Room::area can be renamed")
	}
	count := 0
	for uri, edits := range edit.Changes {
		if uri == "file:///Shape.php" {
			t.Errorf("Shape.php is not edited: %+v", edits)
		}
		count += len(edits)
	}
	if count != 3 {
		t.Errorf("the declaration and the two calls are renamed, got %d edits: %+v", count, edit.Changes)
	}
}

func TestMayMentionNeedsTheNameAsAWord(t *testing.T) {
	method := &symbols.Symbol{Name: "area", Kind: symbols.KindMethod}
	constant := &symbols.Symbol{Name: "AREA", Kind: symbols.KindConstant}
	property := &symbols.Symbol{Name: "$area", Kind: symbols.KindProperty}
	for _, c := range []struct {
		src  string
		sym  *symbols.Symbol
		want bool
	}{
		{"$r->area()", method, true},
		{"$r->areas()", method, false},
		{"$area = 1;", method, false},
		{"$r->size()", method, false},
		{"echo area;", constant, false},
		{"echo AREA;", constant, true},
		{"$r->area", property, true},
		{"public $area;", property, true},
		{"", method, false},
	} {
		if got := mayMention(c.src, c.sym); got != c.want {
			t.Errorf("mayMention(%q, %s) = %v, want %v", c.src, c.sym.Name, got, c.want)
		}
	}
}
