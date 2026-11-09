package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A generic @method or @property type means what the declaring file's
// imports say, not the caller's.
func TestGenericMagicMembersUseDeclaringImports(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Box.php", `<?php
namespace Vendor\Boxes;
/** @template T */
class Box {
    /** @return T */
    public function get() {}
}
class Item { public function label(): string { return ''; } }
`)
	idx.IndexFile("file:///app/Shelf.php", `<?php
namespace App;
use Vendor\Boxes\Box;
use Vendor\Boxes\Item;
/**
 * @method Box<Item> box()
 * @method static Box<Item> make()
 * @property Box<Item> $current
 */
class Shelf {}
`)
	caller := `<?php
namespace Other;
use App\Shelf;
class Box {}
class Item {}
function run(Shelf $s): void {
    $x = 1;
}
`
	p := NewProvider(idx, nil, "")
	file := parser.ParseFile(caller)
	for expr, want := range map[string]string{
		"$s->box()->get()":     "Vendor\\Boxes\\Item",
		"Shelf::make()->get()": "Vendor\\Boxes\\Item",
		"$s->current->get()":   "Vendor\\Boxes\\Item",
		"$s->box()":            "Vendor\\Boxes\\Box<Vendor\\Boxes\\Item>",
	} {
		if got := p.ResolveExpressionTypeTyped(expr, caller, protocol.Position{Line: 6}, file).String(); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}
