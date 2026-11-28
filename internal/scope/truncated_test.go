package scope

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/textpos"
)

const truncatedSource = `<?php
namespace App;
use Foo\{Bar, Baz as Qux};
#[Attr(1)]
final class Box extends Base implements I {
    use T { a as b; }
    public function __construct(private readonly int $size = 1, public ?string $name = null) {}
    public function run(array $items, &$out, ...$rest): array {
        global $first, $second;
        static $count = 0, $seen = [1, 2];
        [$a, [$b, 'k' => $c]] = $items;
        list($d, , $e) = $items;
        foreach ($items as $key => &$value) { $out[$key] = $value; }
        $fn = fn($x) => $x + $a;
        $cl = function ($y) use (&$b, $c): int { return $y; };
        try { $z = $fn(1); } catch (\RuntimeException | \LogicException $ex) { $z = 0; } finally { $w = 1; }
        $s = "v={$a} ${b} $c[0] {$this->size}";
        match (true) { $a > 1 => $b, default => $c };
        return compact('a', 'b') + [$count, $seen, $d, $e, $z, $w, $s, $cl];
    }
}
`

func TestCollectReadsEveryTruncationOfAFile(t *testing.T) {
	for n := range len(truncatedSource) + 1 {
		src := truncatedSource[:n]
		lines := strings.Split(src, "\n")
		inside := func(p textpos.Position) bool {
			return p.Line >= 0 && p.Line < len(lines) && p.Character >= 0 && p.Character <= len(lines[p.Line])
		}
		for _, b := range Collect(src).Bindings {
			if !inside(b.Decl.Start) || !inside(b.Decl.End) {
				t.Fatalf("the first %d bytes: %s is declared at %+v, outside the source", n, b.Name, b.Decl)
			}
		}
	}
}

func TestCollectReadsASnippetAndStrayClosers(t *testing.T) {
	for _, src := range []string{
		"$a = 1;\necho $a;\n",
		"<?php\n$a = 1;\n) {\n    echo $a;\n}\n",
	} {
		doc := Collect(src)
		b := doc.BindingAt(at(t, src, "$a"))
		if b == nil {
			t.Errorf("%q: $a is not bound", src)
			continue
		}
		if got := len(doc.Occurrences(b)); got != 2 {
			t.Errorf("%q: %d occurrences of $a, want the assignment and the read", src, got)
		}
	}
}
