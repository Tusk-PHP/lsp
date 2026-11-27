package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestFunctionCallTypeFollowsPHPsNameRules(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///helpers.php", `<?php
namespace App\Helpers;
class Money {}
function money(int $cents): Money {}
function orphan(): Money {}
/** @return Money[] */
function monies(): array {}
`)
	idx.IndexFile("file:///global.php", "<?php\nclass Plain {}\nfunction money(): Plain {}\n")
	idx.IndexFile("file:///other.php", "<?php\nnamespace Other;\nclass Stray {}\n/** @return Stray[] */\nfunction orphans(): array {}\n")
	src := `<?php
namespace App\Http;
use function App\Helpers\money;
use function App\Helpers\money as cash;
use function App\Helpers\monies;

$imported = money(1);
$importedOtherCase = MONEY(1);
$aliased = cash(1);
$global = \money();
$elsewhere = orphan();
$qualified = \App\Helpers\orphan();
foreach (monies() as $each) {
    echo $each;
}
foreach (orphans() as $stray) {
    echo $stray;
}
`
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	r := NewResolver(idx)
	for variable, want := range map[string]string{
		"$imported":          `App\Helpers\Money`,
		"$importedOtherCase": `App\Helpers\Money`,
		"$aliased":           `App\Helpers\Money`,
		"$global":            "Plain",
		"$elsewhere":         "",
		"$qualified":         `App\Helpers\Money`,
	} {
		pos := textpos.Position{Line: lineContaining(t, lines, variable+" =") + 1}
		if got := r.ResolveVariableTypeTyped(variable, lines, pos, file).FullString(); got != want {
			t.Errorf("%s = %q, want %q", variable, got, want)
		}
		if got := r.ResolveVariableType(variable, lines, pos, file); got != want {
			t.Errorf("untyped %s = %q, want %q", variable, got, want)
		}
	}
	for variable, want := range map[string]string{"$each": `App\Helpers\Money`, "$stray": ""} {
		pos := textpos.Position{Line: lineContaining(t, lines, "echo "+variable)}
		if got := r.ResolveVariableTypeTyped(variable, lines, pos, file).FullString(); got != want {
			t.Errorf("foreach %s = %q, want %q", variable, got, want)
		}
	}
}

func TestForeachOverAFunctionCallTheExpressionTyperCannotRead(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///helpers.php", `<?php
namespace App\Helpers;
class Money {}
/** @return Money[] */
function monies(): array {}
function plain(): array {}
function nothing(): void {}
`)
	src := `<?php
namespace App\Http;
use function App\Helpers\monies;
use function App\Helpers\plain;
use function App\Helpers\nothing;

foreach (monies() ?: [] as $money) {
    echo $money;
}
foreach (plain() ?: [] as $nothing) {
    echo $nothing;
}
foreach (nothing() ?: [] as $void) {
    echo $void;
}
foreach (unknown() ?: [] as $unknown) {
    echo $unknown;
}
`
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	r := NewResolver(idx)
	for variable, want := range map[string]string{
		"$money":   `App\Helpers\Money`,
		"$nothing": "",
		"$void":    "",
		"$unknown": "",
	} {
		pos := textpos.Position{Line: lineContaining(t, lines, "echo "+variable)}
		if got := r.ResolveVariableTypeTyped(variable, lines, pos, file).FullString(); got != want {
			t.Errorf("foreach %s = %q, want %q", variable, got, want)
		}
	}
}

func TestImportedFunctionCallsNarrowAndTypeClosures(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///helpers.php", `<?php
namespace App\Helpers;
class Money {}
/** @phpstan-assert Money $value */
function assertMoney($value): void {}
/** @param callable(Money): void $fn */
function eachMoney(callable $fn): void {}
`)
	src := `<?php
namespace App\Http;
use function App\Helpers\assertMoney;
use function App\Helpers\eachMoney as everyMoney;

function run($checked, $unchecked): void {
    assertMoney($checked);
    unknownAssert($unchecked);
    echo $checked, $unchecked;
    everyMoney(function ($money) {
        echo $money;
    });
    unknownEach(function ($other) {
        echo $other;
    });
}
`
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	r := NewResolver(idx)
	for _, c := range []struct{ variable, line, want string }{
		{"$checked", "echo $checked", `App\Helpers\Money`},
		{"$unchecked", "echo $checked", ""},
		{"$money", "echo $money", `App\Helpers\Money`},
		{"$other", "echo $other", ""},
	} {
		pos := textpos.Position{Line: lineContaining(t, lines, c.line), Character: 4}
		if got := r.ResolveVariableTypeTyped(c.variable, lines, pos, file).FullString(); got != c.want {
			t.Errorf("%s = %q, want %q", c.variable, got, c.want)
		}
	}
}

func TestForeachOverAFunctionDeclaredInTheUnindexedFile(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///item.php", `<?php
namespace App\Models;
class Item {}
`)
	src := `<?php
namespace App;
use App\Models\Item;

/** @return Item[] */
function items(): array {}

foreach (items() as $item) {
    echo $item;
}
foreach (items() ?: [] as $fallback) {
    echo $fallback;
}
`
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	r := NewResolver(idx)
	for _, variable := range []string{"$item", "$fallback"} {
		pos := textpos.Position{Line: lineContaining(t, lines, "echo "+variable)}
		if got := r.ResolveVariableTypeTyped(variable, lines, pos, file).FullString(); got != `App\Models\Item` {
			t.Errorf("foreach %s = %q, want App\\Models\\Item", variable, got)
		}
	}
}
