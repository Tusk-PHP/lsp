package analyzer

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

func TestDefinitionSymfonyCommandName(t *testing.T) {
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, "/project", "symfony")
	a := NewAnalyzer(idx, ca)
	commands := frameworksymfony.NewCommandIndex(phpparser.OnlyGrammar(token.Latest))
	const cmdURI = "file:///project/src/Command/SyncCommand.php"
	commands.UpdateFile(cmdURI, "<?php\nnamespace App\\Command;\nuse Symfony\\Component\\Console\\Attribute\\AsCommand;\n#[AsCommand(name: 'app:sync')]\nclass SyncCommand {}\n")
	a.SetSymfonyCommands(commands)
	src := "<?php\nfunction run(\\Symfony\\Component\\Console\\Application $application, \\Psr\\Container\\ContainerInterface $container) {\n" +
		"$application->find('app:sync');\n$container->get('app:sync');\n$application->find('app:none');\n$application\n    ->find(\n        'app:sync-split'\n    );\n}\n"
	at := func(anchor string) protocol.Position {
		i := strings.Index(src, anchor) + len(anchor) - 3
		return protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1}
	}
	if loc := a.FindDefinition("file:///project/src/x.php", src, at("find('app:sync")); loc == nil || loc.URI != cmdURI || loc.Range.Start.Line != 4 {
		t.Errorf("console application: got %+v", loc)
	}
	if loc := a.FindDefinition("file:///project/src/x.php", src, at("get('app:sync")); loc != nil && loc.URI == cmdURI {
		t.Error("a container's get() went to the console command")
	}
	if loc := a.FindDefinition("file:///project/src/x.php", src, at("find('app:none")); loc != nil && loc.URI == cmdURI {
		t.Error("an unknown command name went to a command")
	}
	commands.UpdateFile("file:///project/src/Command/SplitCommand.php", "<?php\nnamespace App\\Command;\nuse Symfony\\Component\\Console\\Attribute\\AsCommand;\n#[AsCommand(name: 'app:sync-split')]\nclass SplitCommand {}\n")
	if loc := a.FindDefinition("file:///project/src/x.php", src, at("'app:sync-split")); loc == nil || loc.URI != "file:///project/src/Command/SplitCommand.php" {
		t.Errorf("a call split over lines: got %+v", loc)
	}
}
