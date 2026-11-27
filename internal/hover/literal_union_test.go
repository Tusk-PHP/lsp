package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverKeepsEveryLiteralOfAUnion(t *testing.T) {
	src := `<?php
class Ascending {}
class Descending {}
/** @phpstan-assert-if-true 'desc' $value */
function isDescending($value): bool {}
/** @phpstan-assert !'asc' $value */
function notAscending($value): void {}
/**
 * @param 'asc'|'desc' $dir
 * @return ($dir is 'asc' ? Ascending : Descending)
 */
function order(string $dir) {}
/**
 * @template T
 * @param T $value
 * @return T
 */
function same($value) {}
/**
 * @template T of int
 * @param T $value
 * @return T
 */
function sameInt($value) {}
/**
 * @param 'asc'|'desc' $dir
 * @param 1|2 $level
 */
function sort(string $dir, int $level) {
    echo $level;
    $order = order($dir);
    echo $order;
    if (isDescending($dir)) {
        echo $dir;
    }
    $copy = same($level);
    echo $copy;
    $exact = sameInt($level);
    echo $exact;
    notAscending($dir);
    return $dir;
}
/** @param 'asc'|'desc' $dir */
function show(string $dir) {
    print $dir;
}
`
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///sort.php", src)
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	at := func(needle string) protocol.Position {
		i := strings.Index(src, needle)
		if i < 0 {
			t.Fatalf("%q not in source", needle)
		}
		i += strings.Index(needle, "$")
		return protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n")}
	}
	for needle, want := range map[string]string{
		"echo $level;": "1|2 $level",
		"echo $order;": "Ascending|Descending $order",
		"echo $dir;":   "'desc' $dir",
		"echo $copy;":  "int $copy",
		"return $dir;": "'desc' $dir",
		"print $dir;":  "'asc'|'desc' $dir",
	} {
		h := p.GetHover("file:///sort.php", src, at(needle))
		if h == nil || !strings.Contains(h.Contents.Value, "\n"+want+"\n") {
			t.Errorf("hover on %q: want %q, got %+v", needle, want, h)
		}
	}
}
