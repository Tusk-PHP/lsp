package analyzer

import "testing"

const memberKindSource = `<?php
namespace App;
class Room {
    public static int $SIZE = 1;
    const WIDTH = 2;
    public int $WIDTH = 1;
    public static int $DEPTH = 1;
    const SIZE = 2;
}
enum Unit {
    case Metre;
}
function measure(Room $room) {
    echo Room::SIZE;
    echo Room::$SIZE;
    echo $room->WIDTH;
    echo Room::DEPTH;
    echo Unit::Metre;
}
`

func TestDefinitionOfAClassMemberFollowsTheAccessForm(t *testing.T) {
	a, _ := setupRenameAnalyzer(map[string]string{"file:///Room.php": memberKindSource})
	for needle, wantLine := range map[string]int{"SIZE;\n    echo Room::$": 7, "$SIZE;": 3, "WIDTH;\n    echo Room::DEPTH": 5, "Metre;\n}": 10} {
		pos := positionOf(t, memberKindSource, needle)
		if got := a.FindDefinition("file:///Room.php", memberKindSource, pos); got == nil || got.Range.Start.Line != wantLine {
			t.Errorf("definition of %q: got %+v, want line %d", needle, got, wantLine)
		}
	}
	if got := a.FindDefinition("file:///Room.php", memberKindSource, positionOf(t, memberKindSource, "DEPTH;\n    echo Unit")); got != nil {
		t.Errorf("Room::DEPTH names a constant, and Room has only a property $DEPTH: got %+v", got)
	}
}
