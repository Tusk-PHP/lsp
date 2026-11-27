package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestTopLevelVariableTypeDoesNotDependOnLineLayout(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Shop.php", "<?php\nnamespace App;\nclass Shop { public function total(): int { return 1; } }\n")
	r := NewResolver(idx)
	for _, src := range []string{
		"<?php use App\\Shop; $shop = new Shop(); $shop->total();",
		"<?php\nuse App\\Shop; $shop = new Shop();\n$shop->total();",
		"<?php\nuse App\\Shop;\nfoo(); $shop = new Shop();\n$shop->total();",
		"<?php\nuse App\\Shop;\n$shop = new Shop();\n$shop->total();",
	} {
		i := strings.LastIndex(src, "$shop")
		pos := textpos.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1}
		file := parser.ParseFile(src, token.Latest)
		if got := r.ResolveVariableType("$shop", SplitLines(src), pos, file); got != `App\Shop` {
			t.Errorf("%q: $shop = %q, want App\\Shop", src, got)
		}
		if got := r.ResolveVariableTypeTyped("$shop", SplitLines(src), pos, file); got.FQN != `App\Shop` {
			t.Errorf("%q: typed $shop = %q, want App\\Shop", src, got.FQN)
		}
	}
}
