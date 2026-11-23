package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestDefinitionSymfonyCommandName(t *testing.T) {
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, "/project", "symfony")
	a := NewAnalyzer(idx, ca)
	commands := frameworksymfony.NewCommandIndex()
	const cmdURI = "file:///project/src/Command/SyncCommand.php"
	commands.UpdateFile(cmdURI, "<?php\nnamespace App\\Command;\n#[AsCommand(name: 'app:sync')]\nclass SyncCommand {}\n")
	a.SetSymfonyCommands(commands)
	// The receiver's type, as completion would resolve it.
	a.SetChainResolver(func(expr, source string, pos protocol.Position, file *parser.FileNode) string {
		switch expr {
		case "$application":
			return `Symfony\Component\Console\Application`
		case "$container":
			return `Psr\Container\ContainerInterface`
		}
		return ""
	})
	src := "<?php\n$application->find('app:sync');\n$container->get('app:sync');\n$application->find('app:none');\n"
	at := func(anchor string) protocol.Position {
		i := strings.Index(src, anchor) + len(anchor) - 3
		return protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1}
	}
	if loc := a.FindDefinition("file:///project/src/x.php", src, at("find('app:sync")); loc == nil || loc.URI != cmdURI || loc.Range.Start.Line != 2 {
		t.Errorf("console application: got %+v", loc)
	}
	if loc := a.FindDefinition("file:///project/src/x.php", src, at("get('app:sync")); loc != nil && loc.URI == cmdURI {
		t.Error("a container's get() went to the console command")
	}
	if loc := a.FindDefinition("file:///project/src/x.php", src, at("find('app:none")); loc != nil && loc.URI == cmdURI {
		t.Error("an unknown command name went to a command")
	}
}

func TestSourceLine(t *testing.T) {
	src := "zero\r\none\ntwo"
	for n, want := range map[int]string{0: "zero", 1: "one", 2: "two"} {
		if got, ok := sourceLine(src, n); !ok || got != want {
			t.Errorf("line %d: got %q, %v", n, got, ok)
		}
	}
	if _, ok := sourceLine(src, 3); ok {
		t.Error("line past the end")
	}
	if _, ok := sourceLine(src, -1); ok {
		t.Error("negative line")
	}
}
