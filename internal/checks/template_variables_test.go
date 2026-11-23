package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// A Magento-style .phtml template: $block and $escaper come from the renderer.
const magentoTemplate = `<?php
/** @var \Magento\Framework\View\Element\Template $block */
$items = $block->getItems();
$label = fn ($item) => $escaper->escapeHtml( $item->getName() );
?>
<ul>
<?php foreach ( $items as $item ) : ?>
    <li><?= $label( $item ) ?> <?= $escaper->escapeUrl( $block->getUrl() ) ?></li>
<?php endforeach; ?>
</ul>
<?php
function helper() {
    return $block->getId() . $missing;
}
$cb = function () { return $escaper; };
`

// The same code with no HTML: an ordinary script, not a view.
const magentoScript = `<?php
$items = $block->getItems();
$label = fn ($item) => $escaper->escapeHtml( $item->getName() );
foreach ( $items as $item ) {
    echo $label( $item ), $escaper->escapeUrl( $block->getUrl() );
}
function helper() {
    return $block->getId() . $missing;
}
$cb = function () { return $escaper; };
`

func undefinedNames(t *testing.T, src string, included bool) map[string]int {
	t.Helper()
	got := map[string]int{}
	checker := &UndefinedVariableRule{IncludedFile: included}
	for _, f := range checker.Check(parser.ParseFile(src), src, nil) {
		got[strings.TrimPrefix(f.Message, "Undefined variable ")]++
	}
	return got
}

func TestUndefinedVariableInIncludedTemplate(t *testing.T) {
	got := undefinedNames(t, magentoTemplate, true)
	// Top level and the arrow function see the includer's variables; a named
	// function and a full closure don't, so those are still reported.
	want := map[string]int{"$block": 1, "$missing": 1, "$escaper": 1}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for name, n := range want {
		if got[name] != n {
			t.Errorf("%s reported %d times, want %d (all: %v)", name, got[name], n, got)
		}
	}
}

func TestUndefinedVariableInOrdinaryFileStillReportsTopLevel(t *testing.T) {
	got := undefinedNames(t, magentoScript, false)
	if got["$block"] < 3 || got["$escaper"] < 2 {
		t.Errorf("a normal file should report top-level $block and $escaper, got %v", got)
	}
}
