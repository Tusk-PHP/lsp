package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestFindDeclarationGoesToTheInterface(t *testing.T) {
	shapes := `<?php
namespace App;
interface Shape { public function area(): float; }
class Square implements Shape { public function area(): float { return 1.0; } public function side(): float { return 1.0; } }
trait Measures { public function area(): float { return 2.0; } }
class ViaTrait implements Shape { use Measures; }
`
	use := `<?php
namespace App;
function measure(Square $s): float { return $s->area() + $s->side(); }
function viaTrait(ViaTrait $v): float { return $v->area(); }
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///Shape.php": shapes, "file:///use.php": use})
	at := func(needle string) protocol.Position {
		i := strings.Index(use, needle)
		return protocol.Position{Line: strings.Count(use[:i], "\n"), Character: i - strings.LastIndex(use[:i], "\n") - 1}
	}
	line := func(loc *protocol.Location) int {
		if loc == nil {
			return -1
		}
		return loc.Range.Start.Line
	}
	if got := line(a.FindDeclaration("file:///use.php", use, at("area"))); got != 2 {
		t.Errorf("declaration of area: line %d, want the interface on line 2", got)
	}
	if got := line(a.FindDefinition("file:///use.php", use, at("area"))); got != 3 {
		t.Errorf("definition of area: line %d, want the class on line 3", got)
	}
	if got := line(a.FindDeclaration("file:///use.php", use, at("side"))); got != 3 {
		t.Errorf("declaration of a method nothing above declares is its definition: line %d", got)
	}
	if got := line(a.FindDeclaration("file:///use.php", use, at("area(); }"))); got != 2 {
		t.Errorf("declaration of a trait method the class uses: line %d, want the interface the class implements on line 2", got)
	}
	if got := a.FindDeclaration("file:///use.php", use, protocol.Position{Line: 0, Character: 0}); got != nil {
		t.Errorf("nothing under the cursor: got %v", got)
	}
}

func positionOf(t testing.TB, src, needle string) protocol.Position {
	t.Helper()
	i := strings.Index(src, needle)
	if i < 0 {
		t.Fatalf("%q isn't in the source", needle)
	}
	return protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1}
}

func TestFindDeclarationOnAMemberDeclarationNameStaysInItsClass(t *testing.T) {
	shapes := `<?php
namespace App;
interface Shape { public function area(): float; }
class Square implements Shape { public function area(): float { return 1.0; } }
`
	room := `<?php
namespace Other;
class Room {
    public function area(): float { return 2.0; }
}
`
	jobs := `<?php
namespace App\Jobs;
interface ShouldQueue { public function handle(): void; }
class SendMail implements ShouldQueue {
    public function handle(): void {}
}
class Cleanup {
    public function handle(): void { echo 1; }
}
`
	files := map[string]string{"file:///Shape.php": shapes, "file:///Room.php": room, "file:///Jobs.php": jobs}
	a, _ := setupRenameAnalyzer(files)
	cases := []struct {
		uri, needle string
		wantURI     string
		wantLine    int
	}{
		{"file:///Room.php", "area", "file:///Room.php", 3},
		{"file:///Shape.php", "area(): float { return", "file:///Shape.php", 2},
		{"file:///Jobs.php", "handle(): void {}", "file:///Jobs.php", 2},
		{"file:///Jobs.php", "handle(): void { echo", "file:///Jobs.php", 7},
	}
	for _, c := range cases {
		src := files[c.uri]
		pos := positionOf(t, src, c.needle)
		if got := a.FindDeclaration(c.uri, src, pos); got == nil || got.URI != c.wantURI || got.Range.Start.Line != c.wantLine {
			t.Errorf("declaration at %s %q: got %v, want %s line %d", c.uri, c.needle, got, c.wantURI, c.wantLine)
		}
		if got := a.FindDefinition(c.uri, src, pos); got == nil || got.URI != c.uri || got.Range.Start.Line != pos.Line {
			t.Errorf("definition at %s %q: got %v, want the declaration itself", c.uri, c.needle, got)
		}
	}
}

func TestFindDeclarationBeforeReindexIgnoresTheMemberThatUsedToBeThere(t *testing.T) {
	indexed := `<?php
namespace App;
interface Shape { public function area(): float; }
class Square implements Shape {
    public function area(): float { return 1.0; }
    public function size(): float { return 1.0; }
}
`
	edited := `<?php
namespace App;
interface Shape { public function area(): float; }
class Square implements Shape {
    public function size(): float { return 1.0; }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///Shape.php": indexed})
	pos := positionOf(t, edited, "size")
	for name, got := range map[string]*protocol.Location{
		"declaration": a.FindDeclaration("file:///Shape.php", edited, pos),
		"definition":  a.FindDefinition("file:///Shape.php", edited, pos),
	} {
		if got == nil || got.Range.Start.Line != 5 {
			t.Errorf("%s of size where area was: got %v, want size on its indexed line 5", name, got)
		}
	}
}
