package analyzer

import "testing"

func TestCopyReferenceOffersNothingForAnAnonymousClassMember(t *testing.T) {
	src := `<?php
namespace App;
class Room {
    public function area() {}
    public function make() {
        return new class {
            public function area() {}
        };
    }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///Room.php": src})
	if got := a.CopyReference("file:///Room.php", src, positionOf(t, src, "area() {}\n        };")); got != nil {
		t.Errorf("no one can write an anonymous class's name, so there is nothing to copy; got %+v", got)
	}
	if got := a.CopyReference("file:///Room.php", src, positionOf(t, src, "area() {}\n    public function make")); len(got) == 0 {
		t.Error("Room::area still has references to copy")
	}
}
