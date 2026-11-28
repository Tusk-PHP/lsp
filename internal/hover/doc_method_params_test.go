package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverDocMethodWithCommasInParams(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Builder.php", `<?php
namespace App;

/**
 * @method void configure(array{a: int, b: int} $opts, Closure(int, string): bool $cb, array $x = [1, 2], string $s = 'a,b')
 */
class Builder {}
`)
	src := `<?php
namespace App;

function run(Builder $b): void {
    $b->configure([], fn() => true);
}
`
	got := hoverAt(t, NewProvider(idx, nil, "none"), src, "$b->configure", "configure")
	want := "(magic) public function configure(\n    array{a: int, b: int} $opts,\n    Closure(int, string): bool $cb,\n    array $x = [1, 2],\n    string $s = 'a,b'\n): void"
	if !strings.Contains(got, want) {
		t.Errorf("hover = %q, want it to contain %q", got, want)
	}
}
