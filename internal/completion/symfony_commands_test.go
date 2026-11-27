package completion

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestSymfonyCommandNameCompletion(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Application.php", `<?php
namespace Symfony\Component\Console;
class Application { public function find(string $name) {} }
`)
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/project", "symfony"), "symfony")
	commands := frameworksymfony.NewCommandIndex(phpparser.OnlyGrammar(token.Latest))
	commands.UpdateFile("file:///project/src/Command/SyncCommand.php", "<?php\nuse Symfony\\Component\\Console\\Attribute\\AsCommand;\n#[AsCommand(name: 'app:sync', description: 'Sync things')]\nclass SyncCommand {}\n")
	commands.UpdateFile("file:///project/src/Command/OtherCommand.php", "<?php\nuse Symfony\\Component\\Console\\Attribute\\AsCommand;\n#[AsCommand(name: 'other:thing')]\nclass OtherCommand {}\n")
	p.SetSymfonyCommands(commands)

	src := `<?php
use Symfony\Component\Console\Application;
function run(Application $application) {
    $application->find('app:
    $application->find(
}
`
	complete := func(anchor string) []protocol.CompletionItem {
		i := strings.Index(src, anchor) + len(anchor)
		pos := protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1}
		return p.GetCompletions("file:///project/src/run.php", src, pos)
	}
	items := complete("find('app:")
	if len(items) != 1 || items[0].Label != "app:sync" || items[0].Detail != "Sync things" {
		t.Errorf("find('app: offered %+v", items)
	}
	items = complete("    $application->find(")
	var labels []string
	for _, it := range items {
		labels = append(labels, it.Label+"="+it.InsertText)
	}
	if strings.Join(labels, " ") != "app:sync='app:sync' other:thing='other:thing'" {
		t.Errorf("find( with no quote offered %v", labels)
	}
}
