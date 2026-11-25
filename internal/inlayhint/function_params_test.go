package inlayhint

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

const namespacedFunctions = `<?php
namespace Other;
function orphan($stray, $wrong) {}
function money($otherCents, $otherCurrency) {}
`

const helperFunctions = `<?php
namespace App\Helpers;
function money($cents, $currency) {}
`

func paramLabels(hints []protocol.InlayHint, line int) []string {
	var labels []string
	for _, h := range hintsOnLine(hints, line) {
		if h.Kind == protocol.InlayHintKindParameter {
			labels = append(labels, h.Label)
		}
	}
	return labels
}

func TestParamNamesFollowPHPsFunctionNameRules(t *testing.T) {
	p := setupProvider(t, "file:///other.php", namespacedFunctions, "file:///helpers.php", helperFunctions)
	source := `<?php
namespace App\Http;
use function App\Helpers\money;

orphan(1, 2);
money(1, 2);
\Other\money(1, 2);
`
	cfg := allCfg()
	cfg.SuppressSingleParam = false
	cfg.SuppressNameMatch = false
	hints := p.GetInlayHints("file:///test.php", source, cfg)

	if got := paramLabels(hints, lineOf(source, "orphan(")); len(got) != 0 {
		t.Errorf("a function that doesn't exist where it's called gets no hints, got %v", got)
	}
	if got := paramLabels(hints, lineOf(source, "money(1, 2)")); !slices.Equal(got, []string{"cents:", "currency:"}) {
		t.Errorf("the imported function's parameters name the arguments, got %v", got)
	}
	if got := paramLabels(hints, lineOf(source, `\Other\money(`)); !slices.Equal(got, []string{"otherCents:", "otherCurrency:"}) {
		t.Errorf("a fully qualified call names the function it qualifies, got %v", got)
	}
}
