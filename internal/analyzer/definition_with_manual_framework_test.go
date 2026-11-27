package analyzer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func assertEveryDefinitionPathFinds(t *testing.T, a *Analyzer, uri, src string, pos protocol.Position, wantURI string, wantLine int) {
	t.Helper()
	found := map[string]*protocol.Location{
		"definition":             a.FindDefinition(uri, src, pos),
		"definition with manual": a.FindDefinitionWithManual(uri, src, pos, true, "").Location,
		"definition, manual off": a.FindDefinitionWithManual(uri, src, pos, false, "").Location,
		"declaration":            a.FindDeclaration(uri, src, pos),
	}
	for name, loc := range found {
		if loc == nil || loc.URI != wantURI || (wantLine >= 0 && loc.Range.Start.Line != wantLine) {
			t.Errorf("%s: got %+v, want %s line %d", name, loc, wantURI, wantLine)
		}
	}
}

func TestOpenManualKeepsSymfonyCommandRouteAndTranslationDefinitions(t *testing.T) {
	t.Run("command", func(t *testing.T) {
		idx := symbols.NewIndex()
		idx.RegisterBuiltins()
		a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/project", "symfony"))
		commands := frameworksymfony.NewCommandIndex(phpparser.OnlyGrammar(token.Latest))
		const cmdURI = "file:///project/src/Command/SyncCommand.php"
		commands.UpdateFile(cmdURI, "<?php\nnamespace App\\Command;\nuse Symfony\\Component\\Console\\Attribute\\AsCommand;\n#[AsCommand(name: 'app:sync')]\nclass SyncCommand {}\n")
		a.SetSymfonyCommands(commands)
		a.SetChainResolver(func(expr, source string, pos protocol.Position, file *parser.FileNode) string {
			if expr == "$application" {
				return `Symfony\Component\Console\Application`
			}
			return ""
		})
		src := "<?php\n$application->find('app:sync');\n"
		assertEveryDefinitionPathFinds(t, a, "file:///project/src/x.php", src, posOf(t, src, "sync"), cmdURI, 4)
	})

	t.Run("route", func(t *testing.T) {
		a, src := setupSymfonyRouteAnalyzer(t)
		var pos protocol.Position
		for i, line := range strings.Split(src, "\n") {
			if col := strings.Index(line, "product_show"); col >= 0 && strings.Contains(line, "generateUrl") {
				pos = protocol.Position{Line: i, Character: col + 3}
				break
			}
		}
		const uri = "file:///src/Controller/ProductController.php"
		want := a.FindDefinitionWithManual(uri, src, pos, false, "").Location
		if want == nil {
			t.Fatal("the route resolves without the manual")
		}
		assertEveryDefinitionPathFinds(t, a, uri, src, pos, uri, want.Range.Start.Line)
	})

	t.Run("translation key", func(t *testing.T) {
		root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "symfony"))
		if err != nil {
			t.Fatal(err)
		}
		idx := symbols.NewIndex()
		idx.RegisterBuiltins()
		a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, root, "symfony"))
		a.SetSymfonyTranslationResolver(frameworksymfony.NewTranslationResolver(root))
		src := "<?php\n$translator->trans('nav.home');\n"
		want := a.FindDefinitionWithManual("file:///x.php", src, posOf(t, src, "home"), false, "").Location
		if want == nil || !strings.HasSuffix(want.URI, "messages.en.yaml") {
			t.Fatalf("the translation key resolves without the manual: %+v", want)
		}
		assertEveryDefinitionPathFinds(t, a, "file:///x.php", src, posOf(t, src, "home"), want.URI, want.Range.Start.Line)
	})
}
