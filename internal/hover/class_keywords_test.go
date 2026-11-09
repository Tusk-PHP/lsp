package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const classKeywordsSrc = `<?php
namespace App;
abstract class Base {
    protected static int $count = 0;
}
final class Child extends Base {
    private static array $cache = [];
    public function run(): void {
        $e = self::$cache;
        $f = static::$count;
        $g = new self();
        $h = new STATIC();
        $i = new parent();
        $m = self::class;
        $r = $g instanceof self;
    }
}
`

func hoverKeywordAt(t *testing.T, marker string, offset int) string {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Child.php", classKeywordsSrc)
	p := NewProvider(idx, nil, "none")
	for i, line := range strings.Split(classKeywordsSrc, "\n") {
		if col := strings.Index(line, marker); col >= 0 {
			h := p.GetHover("file:///Child.php", classKeywordsSrc, protocol.Position{Line: i, Character: col + offset})
			if h == nil {
				return ""
			}
			return h.Contents.Value
		}
	}
	t.Fatalf("marker %q not found", marker)
	return ""
}

func TestHoverClassKeywordsAndStaticProperties(t *testing.T) {
	cases := []struct {
		marker string
		offset int
		want   string
	}{
		{"self::$cache", len("self::$"), "App\\Child::$cache"},
		{"static::$count", len("static::$"), "App\\Base::$count"},
		{"$g = new self", 1, "Child $g"},
		{"$h = new STATIC", 1, "Child $h"},
		{"$i = new parent", 1, "Base $i"},
		{"$m = self::class", 1, "class-string<Child> $m"},
		{"instanceof self", len("instanceof s"), "App\\Child"},
	}
	for _, c := range cases {
		got := hoverKeywordAt(t, c.marker, c.offset)
		if !strings.Contains(got, c.want) {
			t.Errorf("hover on %q: want %q in %q", c.marker, c.want, got)
		}
	}
}
