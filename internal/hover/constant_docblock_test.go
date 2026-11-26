package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnAConstantShowsItsDocblock(t *testing.T) {
	decl := `<?php
namespace App;
use App\Units\Metre;
/** The app's version. */
const VERSION = '1.0';
class Room {
    /** Width of a standard room in metres. */
    const SIZE = 4;
    /**
     * Names of the rooms.
     * @var list<Metre>
     * @deprecated Use Floor::NAMES.
     */
    public const NAMES = [];
    /** Typed natively. @var string */
    const int FLOORS = 2;
}
enum Unit {
    /** One metre. */
    case Metre;
}
`
	use := "<?php\nnamespace App;\necho Room::SIZE;\necho Room::NAMES;\necho Room::FLOORS;\necho Unit::Metre;\necho VERSION;\n"
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Room.php", decl)
	p := NewProvider(idx, nil, "none")
	lines := strings.Split(use, "\n")
	for line, want := range map[int][]string{
		2: {"Width of a standard room in metres."},
		3: {"Names of the rooms.", "const list<Metre> NAMES", "**Deprecated:** Use Floor::NAMES."},
		4: {"Typed natively.", "const int FLOORS"},
		5: {"One metre."},
		6: {"The app's version."},
	} {
		col := strings.LastIndexAny(lines[line], ":") + 1
		if line == 6 {
			col = len("echo ")
		}
		h := p.GetHover("file:///use.php", use, protocol.Position{Line: line, Character: col})
		if h == nil {
			t.Errorf("hover on %q: none", lines[line])
			continue
		}
		for _, w := range want {
			if !strings.Contains(h.Contents.Value, w) {
				t.Errorf("hover on %q lacks %q:\n%s", lines[line], w, h.Contents.Value)
			}
		}
	}
}
