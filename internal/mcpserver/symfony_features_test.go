package mcpserver

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// TestRuntimeWiresSymfonyTranslationsAndCommands checks that the MCP runtime
// completes and resolves Symfony translation keys and console command names
// on the Symfony fixture.
func TestRuntimeWiresSymfonyTranslationsAndCommands(t *testing.T) {
	svc := sharedSymfonySvc(t)
	rt := svc.runtime()
	if svc.Workspace.SymfonyCommands.Find("app:sync-products") == nil {
		t.Fatal("the console command scan didn't run")
	}
	src := `<?php
use Symfony\Bundle\FrameworkBundle\Console\Application;
use Symfony\Contracts\Translation\TranslatorInterface;
function probe(Application $application, TranslatorInterface $translator) {
    $application->find('app:sync-products');
    $translator->trans('nav.home');
}
`
	uri := "file://" + filepath.Join(svc.RootPath, "src/Probe.php")
	pos := func(anchor string, offset int) protocol.Position {
		i := strings.Index(src, anchor) + offset
		return protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1}
	}
	labels := func(items []protocol.CompletionItem) string {
		var b strings.Builder
		for _, it := range items {
			b.WriteString(it.Label + " ")
		}
		return b.String()
	}
	if got := labels(rt.Completion.GetCompletions(uri, src, pos("app:sync", 0))); !strings.Contains(got, "app:sync-products") {
		t.Errorf("command completion: %s", got)
	}
	if got := labels(rt.Completion.GetCompletions(uri, src, pos("nav.home", 0))); !strings.Contains(got, "nav.home") {
		t.Errorf("translation completion: %s", got)
	}
	if loc := rt.Analyzer.FindDefinition(uri, src, pos("app:sync", 3)); loc == nil || !strings.HasSuffix(loc.URI, "src/Command/SyncProductsCommand.php") {
		t.Errorf("command definition: %+v", loc)
	}
	if loc := rt.Analyzer.FindDefinition(uri, src, pos("nav.home", 3)); loc == nil || !strings.HasSuffix(loc.URI, "translations/messages.en.yaml") {
		t.Errorf("translation definition: %+v", loc)
	}
}
