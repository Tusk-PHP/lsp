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
`
	use := `<?php
namespace App;
function measure(Square $s): float { return $s->area() + $s->side(); }
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
	if got := a.FindDeclaration("file:///use.php", use, protocol.Position{Line: 0, Character: 0}); got != nil {
		t.Errorf("nothing under the cursor: got %v", got)
	}
}
