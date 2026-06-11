package symfony

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestDiscoverCommandsAsCommandAttribute(t *testing.T) {
	idx := symbols.NewIndex()
	src := `<?php
namespace App\Command;

use Symfony\Component\Console\Attribute\AsCommand;
use Symfony\Component\Console\Command\Command;

#[AsCommand(name: 'app:sync-products', description: 'Synchronise the product catalogue')]
class SyncProductsCommand extends Command
{
    protected function execute($input, $output): int
    {
        return Command::SUCCESS;
    }
}
`
	idx.IndexFile("file:///src/Command/SyncProductsCommand.php", src)

	ci := DiscoverCommands(idx)
	if ci == nil {
		t.Fatal("expected non-nil CommandIndex")
	}

	cmd := ci.Find("app:sync-products")
	if cmd == nil {
		t.Fatal("expected app:sync-products command")
	}
	if cmd.Description != "Synchronise the product catalogue" {
		t.Errorf("description = %q", cmd.Description)
	}
	if cmd.ClassFQN != "App\\Command\\SyncProductsCommand" {
		t.Errorf("ClassFQN = %q", cmd.ClassFQN)
	}
}

func TestDiscoverCommandsLegacyDefaultName(t *testing.T) {
	idx := symbols.NewIndex()
	src := `<?php
namespace App\Command;

use Symfony\Component\Console\Command\Command;

class LegacyImportCommand extends Command
{
    protected static $defaultName = 'app:import-legacy';
    protected static $defaultDescription = 'Imports data from the legacy system';

    protected function execute($input, $output): int
    {
        return Command::SUCCESS;
    }
}
`
	idx.IndexFile("file:///src/Command/LegacyImportCommand.php", src)

	ci := DiscoverCommands(idx)
	cmd := ci.Find("app:import-legacy")
	if cmd == nil {
		t.Fatal("expected app:import-legacy command")
	}
	if cmd.Description != "Imports data from the legacy system" {
		t.Errorf("description = %q", cmd.Description)
	}
}

func TestCommandIndexNames(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///src/Command/FooCommand.php", `<?php
namespace App\Command;
use Symfony\Component\Console\Attribute\AsCommand;
use Symfony\Component\Console\Command\Command;
#[AsCommand(name: 'app:foo')]
class FooCommand extends Command {}
`)
	idx.IndexFile("file:///src/Command/BarCommand.php", `<?php
namespace App\Command;
use Symfony\Component\Console\Attribute\AsCommand;
use Symfony\Component\Console\Command\Command;
#[AsCommand(name: 'app:bar')]
class BarCommand extends Command {}
`)

	ci := DiscoverCommands(idx)
	names := ci.Names()
	if len(names) < 2 {
		t.Fatalf("expected at least 2 names, got %d", len(names))
	}
	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["app:foo"] || !nameSet["app:bar"] {
		t.Errorf("names = %v", names)
	}
}

func TestCommandIndexFind(t *testing.T) {
	ci := &CommandIndex{entries: []CommandEntry{{Name: "app:test", ClassFQN: "App\\Command\\TestCommand"}}}
	if e := ci.Find("app:test"); e == nil {
		t.Fatal("expected to find app:test")
	}
	if e := ci.Find("nonexistent"); e != nil {
		t.Errorf("expected nil for nonexistent command")
	}
}

func TestDiscoverCommandsVendorSkipped(t *testing.T) {
	idx := symbols.NewIndex()
	// Commands under /vendor/ should be skipped
	idx.IndexFile("file:///vendor/bundle/Command/MakeCommand.php", `<?php
namespace Bundle\Command;
use Symfony\Component\Console\Attribute\AsCommand;
use Symfony\Component\Console\Command\Command;
#[AsCommand(name: 'bundle:make')]
class MakeCommand extends Command {}
`)
	ci := DiscoverCommands(idx)
	if ci.Find("bundle:make") != nil {
		t.Error("vendor commands should be skipped")
	}
}

func TestDiscoverCommandsFromFixtureFiles(t *testing.T) {
	// Validate the testdata fixture files can be parsed without panicking.
	root := filepath.Join("..", "..", "..", "testdata", "symfony")
	if _, err := os.Stat(root); err != nil {
		t.Skip("testdata/symfony not found")
	}

	idx := symbols.NewIndex()
	cmdDir := filepath.Join(root, "src", "Command")
	filepath.Walk(cmdDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(path)
		idx.IndexFile("file://"+filepath.ToSlash(path), string(data))
		return nil
	})

	ci := DiscoverCommands(idx)
	names := ci.Names()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}

	if !found["app:sync-products"] {
		t.Errorf("expected app:sync-products, got %v", names)
	}
	if !found["app:import-legacy"] {
		t.Errorf("expected app:import-legacy, got %v", names)
	}
}

func TestExtractCommandNameContext(t *testing.T) {
	tests := []struct {
		line      string
		wantPart  string
		wantQuote string
		wantOk    bool
	}{
		{"$application->find('app:sync", "app:sync", "'", true},
		{"$application->find(\"app:sync", "app:sync", "\"", true},
		{"$application->find('", "", "'", true},
		{"$application->get('app:", "app:", "'", true},
		{"$foo->bar('", "", "", false}, // not a recognized pattern... wait, ->run( is
		{"$application->run('cmd", "cmd", "'", true},
		{"someCall(", "", "", false},
	}
	for _, tt := range tests {
		part, quote, ok := ExtractCommandNameContext(tt.line)
		if ok != tt.wantOk {
			t.Errorf("line=%q: ok=%v want=%v", tt.line, ok, tt.wantOk)
			continue
		}
		if !ok {
			continue
		}
		if part != tt.wantPart {
			t.Errorf("line=%q: partial=%q want=%q", tt.line, part, tt.wantPart)
		}
		if quote != tt.wantQuote {
			t.Errorf("line=%q: quote=%q want=%q", tt.line, quote, tt.wantQuote)
		}
	}
}

func TestParseAsCommandBlock(t *testing.T) {
	tests := []struct {
		block   string
		name    string
		desc    string
	}{
		{`#[AsCommand(name: 'app:do-thing', description: 'Does the thing')]`, "app:do-thing", "Does the thing"},
		{`#[AsCommand('app:simple')]`, "app:simple", ""},
		{`#[AsCommand(name: "app:double-quoted")]`, "app:double-quoted", ""},
		{`#[Route('/foo')]`, "", ""}, // not AsCommand — should be empty
	}
	for _, tt := range tests {
		name, desc := parseAsCommandBlock(tt.block)
		if name != tt.name {
			t.Errorf("block=%q: name=%q want=%q", tt.block, name, tt.name)
		}
		if desc != tt.desc {
			t.Errorf("block=%q: desc=%q want=%q", tt.block, desc, tt.desc)
		}
	}
}

func TestCommandIndexNilSafe(t *testing.T) {
	var ci *CommandIndex
	if ci.All() != nil {
		t.Error("nil CommandIndex.All() should return nil")
	}
	if ci.Find("x") != nil {
		t.Error("nil CommandIndex.Find() should return nil")
	}
	if ci.Names() != nil {
		t.Error("nil CommandIndex.Names() should return nil")
	}
}

func TestDiscoverCommandsPositionalAsCommand(t *testing.T) {
	idx := symbols.NewIndex()
	src := `<?php
namespace App\Command;
use Symfony\Component\Console\Attribute\AsCommand;
use Symfony\Component\Console\Command\Command;
#[AsCommand('app:positional', description: 'Positional name')]
class PositionalCommand extends Command {}
`
	idx.IndexFile("file:///src/Command/PositionalCommand.php", src)

	ci := DiscoverCommands(idx)
	cmd := ci.Find("app:positional")
	if cmd == nil {
		t.Fatal("expected app:positional")
	}
	if !strings.Contains(cmd.ClassFQN, "PositionalCommand") {
		t.Errorf("ClassFQN = %q", cmd.ClassFQN)
	}
}
