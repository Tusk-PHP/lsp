package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A catch variable has the type of the class it catches, or the union of
// them, named as the file's imports and namespace say.
func TestCatchVariableType(t *testing.T) {
	src := `<?php
namespace App;
use Other\Failure as Oops;
class NotFound extends \Exception {}
class Denied extends \Exception {}
function run(bool $f): void {
    try {
        work();
    } catch (NotFound $e) {
        echo $e; // single
    } catch (Denied | Oops $e) {
        echo $e; // union
    } catch (\Throwable $t) {
        echo $t; // qualified
    }
    $x = new NotFound();
    try {
        work();
    } catch (Denied $x) {
        echo $x; // rebind
    }
    echo $x; // after
    try {
        work();
    } catch (\Some\Deep\Error $d) {
        echo $d; // deep
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	at := func(v, marker string) string {
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+marker) {
				return r.ResolveVariableTypeTyped(v, lines, protocol.Position{Line: i, Character: strings.Index(l, v) + 2}, file).FullString()
			}
		}
		t.Fatalf("no line marked %q", marker)
		return ""
	}
	for _, c := range []struct{ v, marker, want string }{
		{"$e", "single", `App\NotFound`},
		{"$e", "union", `App\Denied|Other\Failure`},
		{"$t", "qualified", `Throwable`},
		{"$x", "rebind", `App\Denied`},
		{"$x", "after", `App\NotFound|App\Denied`},
		{"$d", "deep", `Some\Deep\Error`},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}
