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

// The @var of an assignment and the @param of a closure come from the doc
// comment the parser attaches to the statement or the closure, never from a
// docblock further up across a plain comment or other code.
func TestAssignmentAndClosureDocComments(t *testing.T) {
	src := `<?php
namespace App;
class Item {}
class Other {}
function run(array $rows): void {
    /** @var Item $plain */
    $before = 1;
    /* a plain comment */
    $plain = make();
    echo $plain; // plain comment above
    /** @var Item $typed */
    $typed = make();
    echo $typed; // docblock above
    /** @var Item */
    $bare = make();
    echo $bare; // bare var
    /** @param Item $row */
    $each = function ($row) {
        echo $row; // closure in a documented statement
    };
    array_map(/** @param Other $o */ function ($o) {
        echo $o; // documented closure
    }, $rows);
    /** @param Item $skip */
    $x = 1;
    // a line comment
    $next = function ($skip) {
        echo $skip; // closure under a line comment
    };
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
		{"$plain", "plain comment above", ``},
		{"$typed", "docblock above", `App\Item`},
		{"$bare", "bare var", `App\Item`},
		{"$row", "closure in a documented statement", `App\Item`},
		{"$o", "documented closure", `App\Other`},
		{"$skip", "closure under a line comment", ``},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}

// A template's variables are declared in a doc comment that ends its PHP
// block, `/** @var Block $block */ ?>`, and read in the markup after it.
func TestTemplateVarDocComment(t *testing.T) {
	src := "<?php\nnamespace App;\nclass Block { public function title(): string { return ''; } }\n/**\n * @var Block $block\n */\n?>\n<h1><?= $block->title() ?></h1>\n<p><?= $other ?></p>\n"
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.phtml", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
	lines := SplitLines(src)
	at := func(line int, v string) string {
		return r.ResolveVariableTypeTyped(v, lines, textpos.Position{Line: line, Character: strings.Index(lines[line], v) + 2}, file).FullString()
	}
	if got := at(7, "$block"); got != `App\Block` {
		t.Errorf("$block = %q, want App\\Block", got)
	}
	if got := at(8, "$other"); got != "" {
		t.Errorf("$other = %q, want nothing", got)
	}
}

// The `@var`s on a `global` statement, one doc comment each, type the
// variables it brings in, as the header of a Blade view declares what the
// view receives.
func TestGlobalVarDocComment(t *testing.T) {
	src := "<?php /** @var \\App\\User $user */ /** @var string $tab */ global $user, $tab;\necho $user->fullName();\necho $tab;\n"
	idx := symbols.NewIndex()
	idx.IndexFile("file:///User.php", "<?php\nnamespace App;\nclass User { public function fullName(): string { return ''; } }\n")
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
	lines := SplitLines(src)
	at := func(line int, v string) string {
		return r.ResolveVariableTypeTyped(v, lines, textpos.Position{Line: line, Character: strings.Index(lines[line], v) + 2}, file).FullString()
	}
	if got := at(1, "$user"); got != `App\User` {
		t.Errorf("$user = %q, want App\\User", got)
	}
	if got := at(2, "$tab"); got != "string" {
		t.Errorf("$tab = %q, want string", got)
	}
}
