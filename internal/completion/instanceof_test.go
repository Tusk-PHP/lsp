package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// `$o->` where an instanceof test says what $o is completes that class's
// members: in the if block, and after a guard that leaves.
func TestCompleteAfterInstanceof(t *testing.T) {
	src := `<?php
namespace App;

class Foo { public function fooOnly(): int { return 1; } }

class Cases
{
    public function run(object $o, object $g): void
    {
        if ($o instanceof Foo) {
            $o->
        }
        if (!$g instanceof Foo) {
            return;
        }
        $g->
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Cases.php", src)
	p := NewProvider(idx, nil, "")
	for _, pos := range []protocol.Position{{Line: 10, Character: 16}, {Line: 15, Character: 12}} {
		found := false
		for _, item := range p.GetCompletions("file:///Cases.php", src, pos) {
			if item.Label == "fooOnly" {
				found = true
			}
		}
		if !found {
			t.Errorf("at %d:%d: fooOnly not offered", pos.Line, pos.Character)
		}
	}
}
