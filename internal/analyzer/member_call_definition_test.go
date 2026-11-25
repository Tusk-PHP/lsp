package analyzer

import "testing"

func TestDefinitionOfAMemberSharedByAMethodAndAPropertyFollowsTheCall(t *testing.T) {
	src := `<?php
namespace App;
class Room {
    private float $area = 0.0;
    public function area(): float { return $this->area; }
    public function twice(): float { return $this->area() * 2; }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///Room.php": src})
	for needle, wantLine := range map[string]int{"area() * 2": 4, "area; }": 3} {
		pos := positionOf(t, src, needle)
		if got := a.FindDefinition("file:///Room.php", src, pos); got == nil || got.Range.Start.Line != wantLine {
			t.Errorf("definition of %q: got %+v, want line %d", needle, got, wantLine)
		}
		if got := a.FindDefinitionWithManual("file:///Room.php", src, pos, true, "").Location; got == nil || got.Range.Start.Line != wantLine {
			t.Errorf("definition with the manual of %q: got %+v, want line %d", needle, got, wantLine)
		}
	}
}
