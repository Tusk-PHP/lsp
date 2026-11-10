package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Go to definition on a variable goes to its class: a generic's class without
// its type arguments, and for a variable that may hold either of two classes,
// the first of them.
func TestDefinitionVariableGenericAndUnion(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///app/Foo.php", `<?php
namespace App;
class Foo {}
`)
	idx.IndexFile("file:///app/Bar.php", `<?php
namespace App;
class Bar {}
`)
	idx.IndexFile("file:///app/Box.php", `<?php
namespace App;
/** @template T */
class Box {
    /** @param T $v */
    public function __construct(public mixed $v) {}
}
`)
	src := `<?php
namespace App;
function run(bool $c): void {
    $q = new Box(new Foo());
    echo $q;
    if ($c) {
        $x = new Foo();
    } else {
        $x = new Bar();
    }
    echo $x;
}
`
	idx.IndexFile("file:///app/run.php", src)
	a := NewAnalyzer(idx, nil)
	for _, tc := range []struct{ target, line, want string }{
		{"$q", "echo $q", "Box.php"},
		{"$x", "echo $x", "Foo.php"},
	} {
		pos := charPosOf(t, src, tc.target, tc.line)
		loc := a.FindDefinition("file:///app/run.php", src, pos)
		if loc == nil {
			t.Errorf("%s: no definition, want %s", tc.target, tc.want)
			continue
		}
		if !strings.HasSuffix(loc.URI, tc.want) {
			t.Errorf("%s: definition in %s, want %s", tc.target, loc.URI, tc.want)
		}
	}
}
