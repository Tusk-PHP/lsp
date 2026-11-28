package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestAnAssignmentInANestedFunctionDoesNotTypeTheOuterVariable(t *testing.T) {
	src := `<?php
namespace App;
class User {}
class Order {}
class Repo {
    /** @return list<User> */
    public function all(): array { return []; }
}
function run(Repo $repo): void {
    $cb = function () use ($repo) {
        $inner = new User();
        foreach ($repo->all() as $order) {
            echo $order;
        }
        $later = new User();
    };
    $fn = fn() => $arrow = new User();
    echo $inner; // inner
    echo $order; // order
    echo $arrow; // arrow
    $later = new Order();
    echo $later; // reassigned
}
$top = function () {
    $script = new User();
};
echo $script; // top level
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///scope.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	at := func(v, marker string) textpos.Position {
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+marker) {
				return textpos.Position{Line: i, Character: strings.Index(l, v) + 2}
			}
		}
		t.Fatalf("no line marked %q", marker)
		return textpos.Position{}
	}
	for _, c := range []struct{ v, marker, want string }{
		{"$inner", "inner", ""},
		{"$order", "order", ""},
		{"$arrow", "arrow", ""},
		{"$script", "top level", ""},
		{"$later", "reassigned", `App\Order`},
	} {
		pos := at(c.v, c.marker)
		if got := r.ResolveVariableTypeTyped(c.v, lines, pos, file).FullString(); got != c.want {
			t.Errorf("typed %s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
		if got := r.ResolveVariableType(c.v, lines, pos, file); got != c.want {
			t.Errorf("plain %s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}
