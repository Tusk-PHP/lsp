package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

const classKeywordsSrc = `<?php
namespace App;
abstract class Base {
    protected static int $count = 0;
    public function __construct(string $n) {}
}
final class Child extends Base {
    private static array $cache = [];
    public function run(): void {
        parent::__construct('n');
        $e = self::$cache;
        $f = static::$count;
        $g = new self('n');
        $i = new parent('n');
        $r = $g instanceof SELF;
    }
}
`

// at returns the position of the n-th character of marker on the line that
// contains it.
func at(t *testing.T, marker string, offset int) protocol.Position {
	t.Helper()
	for i, line := range strings.Split(classKeywordsSrc, "\n") {
		if col := strings.Index(line, marker); col >= 0 {
			return protocol.Position{Line: i, Character: col + offset}
		}
	}
	t.Fatalf("marker %q not found", marker)
	return protocol.Position{}
}

func TestDefinitionOfClassKeywordsAndStaticProperties(t *testing.T) {
	const uri = "file:///project/Child.php"
	a, src := setupBuiltinAnalyzer(t, uri, classKeywordsSrc)
	cases := []struct {
		name     string
		pos      protocol.Position
		wantLine int // 0-based line of the declaration
	}{
		{"self::$cache", at(t, "self::$cache", len("self::$")), 7},
		{"static::$count", at(t, "static::$count", len("static::$")), 3},
		{"parent keyword", at(t, "parent::__construct", 1), 2},
		{"new self", at(t, "new self", len("new s")), 6},
		{"new parent", at(t, "new parent", len("new p")), 2},
		{"instanceof SELF", at(t, "instanceof SELF", len("instanceof S")), 6},
	}
	for _, c := range cases {
		// The plain path and the php.net-manual path resolve symbols separately.
		for _, manual := range []bool{false, true} {
			var loc *protocol.Location
			if manual {
				loc = a.FindDefinitionWithManual(uri, src, c.pos, true, "").Location
			} else {
				loc = a.FindDefinition(uri, src, c.pos)
			}
			if loc == nil {
				t.Errorf("%s (manual=%v): no definition", c.name, manual)
				continue
			}
			if loc.Range.Start.Line != c.wantLine {
				t.Errorf("%s (manual=%v): line %d, want %d", c.name, manual, loc.Range.Start.Line, c.wantLine)
			}
		}
	}
}
