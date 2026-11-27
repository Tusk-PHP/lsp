package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// An inline `@var Type $name` on a statement that doesn't assign the variable
// gives it that type from there on, in the blocks the statement is in, as
// WooCommerce writes it over a loop's item.
func TestInlineVarAssertion(t *testing.T) {
	src := `<?php
namespace App;
class Item {}
class ProductItem extends Item {}
class Other {}
class Order {
    /** @return Item[] */
    public function items(): array { return []; }
}
function run(Order $order, bool $c): void {
    foreach ($order->items() as $item) {
        /**
         * The product of the item.
         *
         * @var ProductItem $item
         * @var Other $product
         */
        $product = $item->get(); // own statement
        echo $item; // later statement
        if ($c) {
            echo $item; // nested block
        }
    }
    foreach ($order->items() as $raw) {
        /** @var Other $raw */
        $raw
            = $raw->get(); // its own assignment
    }
    foreach ($order->items() as $plain) {
        /** @var ProductItem */
        echo $plain; // bare var
        /** @var ProductItem $other */
        echo $plain; // another variable
    }
    foreach ($order->items() as $branch) {
        if ($c) {
            /** @var ProductItem $branch */
            echo $branch;
        }
        echo $branch; // after another branch
    }
    /** @var ProductItem $fresh */
    work();
    $fresh = new Other();
    echo $fresh; // written after the docblock
    /** @var ProductItem $comment */
    work();
    $comment = new Other();
    /* a plain comment */
    echo $comment; // under a plain comment
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
				return r.ResolveVariableTypeTyped(v, lines, textpos.Position{Line: i, Character: strings.Index(l, v) + 2}, file).FullString()
			}
		}
		t.Fatalf("no line marked %q", marker)
		return ""
	}
	for _, c := range []struct{ v, marker, want string }{
		{"$item", "own statement", `App\ProductItem`},
		{"$item", "later statement", `App\ProductItem`},
		{"$item", "nested block", `App\ProductItem`},
		{"$raw", "its own assignment", `App\Item`},
		{"$plain", "bare var", `App\Item`},
		{"$plain", "another variable", `App\Item`},
		{"$branch", "after another branch", `App\Item`},
		{"$fresh", "written after the docblock", `App\Other`},
		{"$comment", "under a plain comment", `App\Other`},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}
