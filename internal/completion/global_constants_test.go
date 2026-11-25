package completion

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestStandaloneCompletionOffersReachableGlobalConstants(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///config.php", "<?php\nconst APP_VENDOR = 'acme';\nfunction app_version() {}\n", symbols.SourceProject)
	idx.IndexFileWithSource("file:///shop.php", "<?php\nnamespace App\\Shop;\nconst APP_TAX = 0.2;\n", symbols.SourceProject)
	idx.IndexFileWithSource("file:///billing.php", "<?php\nnamespace App\\Billing;\nconst APP_CURRENCY = 'EUR';\nconst APP_HIDDEN = 1;\n", symbols.SourceProject)
	idx.IndexFileWithSource("file:///box.php", "<?php\nclass Box { const APP_SIZE = 3; }\n", symbols.SourceProject)
	p := NewProvider(idx, nil, "")
	src := "<?php\nnamespace App\\Shop;\nuse const App\\Billing\\APP_CURRENCY as APP_MONEY;\n\necho APP_"
	lines := strings.Split(src, "\n")
	pos := protocol.Position{Line: len(lines) - 1, Character: len(lines[len(lines)-1])}
	got := map[string]protocol.CompletionItem{}
	for _, item := range p.GetCompletions("file:///app/Shop/Cart.php", src, pos) {
		got[item.Label] = item
	}
	for label, detail := range map[string]string{"APP_VENDOR": "'acme'", "APP_TAX": "0.2", "APP_MONEY": "'EUR'"} {
		item, ok := got[label]
		if !ok {
			t.Errorf("%s is offered", label)
			continue
		}
		if item.Kind != protocol.CompletionItemKindConstant || item.Detail != detail {
			t.Errorf("%s: kind %d, detail %q; want a constant with %q", label, item.Kind, item.Detail, detail)
		}
	}
	for _, label := range []string{"APP_HIDDEN", "APP_CURRENCY", "APP_SIZE"} {
		if _, ok := got[label]; ok {
			t.Errorf("%s is not reachable by its bare name here", label)
		}
	}
	if _, ok := got["app_version"]; !ok {
		t.Error("functions are still offered")
	}
}
