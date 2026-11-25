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

func TestReferencesFromAByReferenceMethodDeclaration(t *testing.T) {
	src := `<?php
namespace App;
class Room {
    public function &area(): array { static $a = []; return $a; }
    public function twice(): array { return $this->area(); }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///Room.php": src})
	pos := positionOf(t, src, "area(): array")
	if got := a.FindReferences("file:///Room.php", src, pos); len(got) != 2 {
		t.Errorf("references from the declaration of &area: got %+v, want the declaration and the call", got)
	}
	if got := a.Rename("file:///Room.php", src, pos, "size", nil); got == nil || len(got.Changes["file:///Room.php"]) != 2 {
		t.Errorf("rename from the declaration of &area: got %+v, want both names edited", got)
	}
}
