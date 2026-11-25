package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
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
		pos := protocol.Position{Line: lineContaining(t, lines, variable+" =") + 1}
		if got := r.ResolveVariableTypeTyped(variable, lines, pos, file).FullString(); got != want {
			t.Errorf("%s = %q, want %q", variable, got, want)
		}
		if got := r.ResolveVariableType(variable, lines, pos, file); got != want {
			t.Errorf("untyped %s = %q, want %q", variable, got, want)
		}
	}
	for variable, want := range map[string]string{"$each": `App\Helpers\Money`, "$stray": ""} {
		pos := protocol.Position{Line: lineContaining(t, lines, "echo "+variable)}
		if got := r.ResolveVariableTypeTyped(variable, lines, pos, file).FullString(); got != want {
			t.Errorf("foreach %s = %q, want %q", variable, got, want)
		}
	}
}
