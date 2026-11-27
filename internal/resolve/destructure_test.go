package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A variable written by a destructuring has whatever element it takes, which
// isn't known; it doesn't keep the type of an earlier write, as the foreach
// variable's before `[$name, $route] = $route;`.
func TestDestructuredVariableDoesNotKeepEarlierType(t *testing.T) {
	src := `<?php
namespace App;
class Route { public function compile(): void {} }
class Prefix {}
class Holder {
    /** @param array<Prefix> $routes */
    public function run(array $routes): void {
        foreach ($routes as $route) {
            echo $route; // before
            [$name, $route] = $route;
            echo $route; // destructured
        }
        foreach ($routes as $other) {
            list($a, $other) = $other;
            echo $other; // list
        }
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
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
		{"$route", "before", `App\Prefix`},
		{"$route", "destructured", ``},
		{"$other", "list", ``},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}
