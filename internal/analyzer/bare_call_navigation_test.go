package analyzer

import (
	"strings"
	"testing"
)

func TestBareCallNavigationFindsOnlyFunctions(t *testing.T) {
	room := `<?php
namespace Other;
class Room {
    public function area(): float { return 1.0; }
    public function measure(): float { return area() + perimeter(1, 2); }
}
`
	helpers := `<?php
function perimeter(int $w, int $h): float { return 0.0; }
`
	sources := map[string]string{"file:///Room.php": room, "file:///helpers.php": helpers}
	a, read := setupRenameAnalyzer(sources)
	bare := positionOf(t, room, "area() +")
	called := positionOf(t, room, "perimeter(1")

	if got := a.FindAllReferences("file:///Room.php", room, bare, read); len(got) != 0 {
		t.Errorf("references of a bare call with no such function: got %v, want none", got)
	}
	if got := a.PrepareRename("file:///Room.php", room, bare); got != nil {
		t.Errorf("rename of a bare call with no such function: got %+v, want none", got)
	}
	if got := a.Rename("file:///Room.php", room, bare, "size", read); got != nil && len(got.Changes) != 0 {
		t.Errorf("rename of a bare call renames nothing, got %+v", got)
	}

	refs := a.FindAllReferences("file:///Room.php", room, called, read)
	var uris []string
	for _, r := range refs {
		uris = append(uris, r.URI)
	}
	if !strings.Contains(strings.Join(uris, " "), "file:///helpers.php") || !strings.Contains(strings.Join(uris, " "), "file:///Room.php") {
		t.Errorf("references of a called function include its declaration and the call, got %v", refs)
	}
	if got := a.PrepareRename("file:///Room.php", room, called); got == nil {
		t.Error("a called function can be renamed")
	}
}

func TestSignatureHelpOfABareCallIsOnlyAFunction(t *testing.T) {
	room := `<?php
namespace Other;
class Room {
    public function area(int $scale): float { return 1.0; }
    public function measure(): float {
        $r = new Room();
        area(
        perimeter(
        $r->area(
    }
}
`
	helpers := `<?php
function perimeter(int $w, int $h): float { return 0.0; }
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///Room.php": room, "file:///helpers.php": helpers})
	after := func(needle string) int {
		p := positionOf(t, room, needle)
		p.Character += len(needle)
		if help := a.GetSignatureHelp("file:///Room.php", room, p); help != nil {
			return len(help.Signatures[0].Parameters)
		}
		return -1
	}
	if got := after("  area("); got != -1 {
		t.Errorf("a bare call with no such function has no signature, got %d parameters", got)
	}
	if got := after("perimeter("); got != 2 {
		t.Errorf("a called function's signature, got %d parameters", got)
	}
	if got := after("$r->area("); got != 1 {
		t.Errorf("a method's signature through its receiver, got %d parameters", got)
	}
}

func TestSignatureHelpInsideAContainerCallIsTheInnerCall(t *testing.T) {
	caller := `<?php
namespace App;
class Service {}
$s = app(Service::class, ['x' => perimeter(1,
`
	helpers := `<?php
function perimeter(int $w, int $h): float { return 0.0; }
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///caller.php": caller, "file:///helpers.php": helpers})
	pos := positionOf(t, caller, "perimeter(1,")
	pos.Character += len("perimeter(1,")
	help := a.GetSignatureHelp("file:///caller.php", caller, pos)
	if help == nil || !strings.HasPrefix(help.Signatures[0].Label, "perimeter(int $w, int $h)") || help.ActiveParameter != 1 {
		t.Errorf("signature help for perimeter( inside app(...): got %+v", help)
	}
}

func TestMemberOfAnUnresolvedReceiverIsNotNavigated(t *testing.T) {
	room := `<?php
class Room {
    public function area(): float { return 1.0; }
}
function measure($unknown) { return $unknown->area(); }
`
	a, read := setupRenameAnalyzer(map[string]string{"file:///Room.php": room})
	pos := positionOf(t, room, "area(); }")

	if got := a.FindAllReferences("file:///Room.php", room, pos, read); len(got) != 0 {
		t.Errorf("references of a member on an unresolved receiver: got %v, want none", got)
	}
	if got := a.PrepareRename("file:///Room.php", room, pos); got != nil {
		t.Errorf("rename of a member on an unresolved receiver: got %+v, want none", got)
	}
}
