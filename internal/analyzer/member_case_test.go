package analyzer

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const roomSource = `<?php
namespace App;
class Room {
    public $area;
    const SIZE = 1;
    public function area(): int { return 1; }
}
function use_room(Room $room) {
    $room->AREA();
    $room->area;
    Room::SIZE;
    Room::size;
}
`

func roomAnalyzer() *Analyzer {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Room.php", roomSource)
	return NewAnalyzer(idx, nil)
}

func TestDefinitionOfAMethodCalledInAnotherCase(t *testing.T) {
	a := roomAnalyzer()
	for _, tc := range []struct {
		line, word string
		want       int
	}{
		{"$room->AREA()", "AREA", 5},
		{"$room->area;", "area", 3},
		{"Room::SIZE", "SIZE", 4},
	} {
		loc := a.FindDefinition("file:///app/Room.php", roomSource, charPosOf(t, roomSource, tc.word, tc.line))
		if loc == nil || loc.Range.Start.Line != tc.want {
			t.Errorf("definition of %s opens line %d, got %+v", tc.line, tc.want, loc)
		}
	}
	if loc := a.FindDefinition("file:///app/Room.php", roomSource, charPosOf(t, roomSource, "size", "Room::size")); loc != nil {
		t.Errorf("a constant matches only in its own case, got %+v", loc)
	}
}

func TestHighlightsOfAMethodIgnoreCase(t *testing.T) {
	a := roomAnalyzer()
	var lines []int
	for _, h := range a.GetDocumentHighlights("file:///app/Room.php", roomSource, charPosOf(t, roomSource, "AREA", "$room->AREA()")) {
		lines = append(lines, h.Range.Start.Line)
	}
	slices.Sort(lines)
	if !slices.Equal(lines, []int{5, 8}) {
		t.Errorf("the method's declaration and its call in another case are highlighted, not the property: lines %v", lines)
	}
	var decl protocol.DocumentHighlightKind
	for _, h := range a.GetDocumentHighlights("file:///app/Room.php", roomSource, charPosOf(t, roomSource, "AREA", "$room->AREA()")) {
		if h.Range.Start.Line == 5 {
			decl = h.Kind
		}
	}
	if decl != protocol.DocumentHighlightKindWrite {
		t.Errorf("the declaration is the write, got %v", decl)
	}
}

func TestHighlightsOfAClassIgnoreCase(t *testing.T) {
	source := roomSource + "$r = new ROOM();\n"
	a := roomAnalyzer()
	var lines []int
	for _, h := range a.GetDocumentHighlights("file:///app/Room.php", source, charPosOf(t, source, "Room", "function use_room")) {
		lines = append(lines, h.Range.Start.Line)
	}
	slices.Sort(lines)
	if !slices.Equal(lines, []int{2, 7, 10, 11, 13}) {
		t.Errorf("the class's declaration and every use in any case are highlighted: lines %v", lines)
	}
}

func TestDefinitionOfAClassWrittenInAnotherCase(t *testing.T) {
	source := roomSource + "$r = new ROOM();\nroom::SIZE;\n"
	a := roomAnalyzer()
	for _, line := range []string{"new ROOM()", "room::SIZE"} {
		word := "ROOM"
		if line == "room::SIZE" {
			word = "room"
		}
		if loc := a.FindDefinition("file:///app/use.php", source, charPosOf(t, source, word, line)); loc == nil || loc.Range.Start.Line != 2 {
			t.Errorf("definition of %s opens class Room, got %+v", line, loc)
		}
	}
	if loc := a.FindDefinition("file:///app/use.php", source, charPosOf(t, source, "SIZE", "room::SIZE")); loc == nil || loc.Range.Start.Line != 4 {
		t.Errorf("a constant on a class written in another case resolves, got %+v", loc)
	}
}

func highlightLines(a *Analyzer, uri, source string, pos protocol.Position) []int {
	var lines []int
	for _, h := range a.GetDocumentHighlights(uri, source, pos) {
		lines = append(lines, h.Range.Start.Line)
	}
	slices.Sort(lines)
	return lines
}

func TestHighlightsOfAClassSkipMembersAndFunctionsOfTheSameName(t *testing.T) {
	source := roomSource + "$room->room;\nRoom::ROOM;\nfunction room() {}\n"
	got := highlightLines(roomAnalyzer(), "file:///app/Room.php", source, charPosOf(t, source, "Room", "function use_room"))
	if want := []int{2, 7, 10, 11, 14}; !slices.Equal(got, want) {
		t.Errorf("a property, a constant and a function named like the class are not highlighted: lines %v, want %v", got, want)
	}
}

func TestHighlightsOfAConstantMatchItsCaseAndSkipCallsAndProperties(t *testing.T) {
	source := `<?php
namespace App;
class Room {
    const SIZE = 1;
    public function area(): int { return self::SIZE; }
}
function use_room(Room $room) {
    Room::SIZE;
    Room::size;
    $room->SIZE;
    SIZE();
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Room.php", source)
	got := highlightLines(NewAnalyzer(idx, nil), "file:///app/Room.php", source, charPosOf(t, source, "SIZE", "Room::SIZE"))
	if want := []int{3, 4, 7}; !slices.Equal(got, want) {
		t.Errorf("the constant's declaration and uses in its own case are highlighted: lines %v, want %v", got, want)
	}
}
