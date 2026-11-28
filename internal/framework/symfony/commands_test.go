package symfony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
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
	ci := NewCommandIndex(phpparser.OnlyGrammar(token.Latest))
	ci.UpdateFile("file:///src/Command/TestCommand.php", "<?php\nnamespace App\\Command;\nuse Symfony\\Component\\Console\\Attribute\\AsCommand;\n#[AsCommand(name: 'app:test')]\nclass TestCommand {}\n")
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

func TestAsCommandAttributes(t *testing.T) {
	src := `<?php
namespace App\Command;
use Symfony\Component\Console\Attribute\AsCommand;
use Other\AsCommand as NotSymfony;

#[AsCommand(name: 'app:do-thing', description: 'Does the thing')]
class DoThing {}
#[AsCommand('app:simple', 'Positional')]
class Simple {}
#[AsCommand(name: "app:double-quoted")]
class DoubleQuoted {}
#[\Symfony\Component\Console\Attribute\AsCommand(
    name: 'app:multi-line',
)]
class MultiLine {}
#[NotSymfony('app:other')]
class Other {}
#[AsCommand(name: self::NAME)]
class Dynamic {}
class Legacy {
    protected static $defaultName = 'app:legacy';
    protected static $defaultDescription = 'Old style';
}
class Unnamed {
    protected static $defaultName = '';
}
`
	var got []string
	for _, e := range discoverCommandsInFile("file:///c.php", src, phpparser.OnlyGrammar(token.Latest)) {
		got = append(got, fmt.Sprintf("%s %q %s %d:%d-%d", e.Name, e.Description, e.ClassFQN, e.Range.Start.Line, e.Range.Start.Character, e.Range.End.Character))
	}
	want := []string{
		`app:do-thing "Does the thing" App\Command\DoThing 6:6-13`,
		`app:simple "Positional" App\Command\Simple 8:6-12`,
		`app:double-quoted "" App\Command\DoubleQuoted 10:6-18`,
		`app:multi-line "" App\Command\MultiLine 14:6-15`,
		`app:legacy "Old style" App\Command\Legacy 20:21-33`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s", strings.Join(got, "\n"))
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

func TestCommandNameCall(t *testing.T) {
	for _, tt := range []struct {
		prefix, receiver, partial, quote string
		ok                               bool
	}{
		{"$application->find('app:sy", "$application", "app:sy", "'", true},
		{`    $this->application->get("`, "$this->application", "", `"`, true},
		{"$kernel->getApplication()->has(", "$kernel->getApplication()", "", "", true},
		{"$app?->find('a", "$app", "a", "'", true},
		{"$application->find('done')", "", "", "", false},
		{"$application->find($name", "", "", "", false},
		{"$application->run('x", "", "", "", false},
		{"find('x", "", "", "", false},
	} {
		receiver, partial, quote, ok := CommandNameCall(tt.prefix)
		if ok != tt.ok || receiver != tt.receiver || partial != tt.partial || quote != tt.quote {
			t.Errorf("CommandNameCall(%q) = %q, %q, %q, %v; want %q, %q, %q, %v",
				tt.prefix, receiver, partial, quote, ok, tt.receiver, tt.partial, tt.quote, tt.ok)
		}
	}
}

func TestCommandNameAt(t *testing.T) {
	line := `$tester = new CommandTester($application->find('app:sync-products'));`
	at := strings.Index(line, "sync")
	receiver, name, ok := CommandNameAt(line, at)
	if !ok || receiver != "$application" || name != "app:sync-products" {
		t.Errorf("got %q, %q, %v", receiver, name, ok)
	}
	if _, _, ok := CommandNameAt(line, strings.Index(line, "CommandTester")); ok {
		t.Error("cursor outside the argument")
	}
	if _, _, ok := CommandNameAt(line, len(line)+1); ok {
		t.Error("cursor past the line")
	}
}

func TestIsConsoleApplication(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/FrameworkApplication.php", `<?php
namespace Symfony\Bundle\FrameworkBundle\Console;
use Symfony\Component\Console\Application as BaseApplication;
class Application extends BaseApplication {}
`)
	for fqn, want := range map[string]bool{
		`Symfony\Component\Console\Application`:              true,
		`\Symfony\Component\Console\Application`:             true,
		`Symfony\Bundle\FrameworkBundle\Console\Application`: true,
		`Symfony\Component\DependencyInjection\Container`:    false,
		``: false,
	} {
		if got := IsConsoleApplication(idx, fqn); got != want {
			t.Errorf("IsConsoleApplication(%q) = %v, want %v", fqn, got, want)
		}
	}
}

func TestCommandIndexUpdateFile(t *testing.T) {
	ci := NewCommandIndex(phpparser.OnlyGrammar(token.Latest))
	const uri = "file:///src/Command/A.php"
	ci.UpdateFile(uri, "<?php\nuse Symfony\\Component\\Console\\Attribute\\AsCommand;\n#[AsCommand(name: 'app:a')]\nclass A {}\n")
	ci.UpdateFile("file:///vendor/x/B.php", "<?php\nuse Symfony\\Component\\Console\\Attribute\\AsCommand;\n#[AsCommand(name: 'vendor:b')]\nclass B {}\n")
	if got := strings.Join(ci.Names(), ","); got != "app:a" {
		t.Fatalf("names = %q", got)
	}
	ci.UpdateFile(uri, "<?php\nuse Symfony\\Component\\Console\\Attribute\\AsCommand;\n#[AsCommand(name: 'app:renamed')]\nclass A {}\n")
	if got := strings.Join(ci.Names(), ","); got != "app:renamed" {
		t.Errorf("after rename: names = %q", got)
	}
	ci.UpdateFile(uri, "<?php\nclass A {}\n")
	if len(ci.All()) != 0 {
		t.Errorf("after removing the attribute: %v", ci.All())
	}
	other := NewCommandIndex(phpparser.OnlyGrammar(token.Latest))
	other.UpdateFile(uri, "<?php\nuse Symfony\\Component\\Console\\Attribute\\AsCommand;\n#[AsCommand(name: 'app:z')]\nclass A {}\n")
	ci.ReplaceWith(other)
	if ci.Find("app:z") == nil {
		t.Error("ReplaceWith lost app:z")
	}
	var nilIndex *CommandIndex
	nilIndex.UpdateFile(uri, "")
	if nilIndex.All() != nil || nilIndex.Names() != nil || nilIndex.Find("x") != nil {
		t.Error("a nil index should be empty")
	}
}

func TestDiscoverCommandsReadsUnindexedSourcesWithTheProjectGrammar(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "composer.json"), "{}\n")
	kernel := filepath.Join(root, "src", "Kernel.php")
	mustWriteFile(t, kernel, "<?php\nnamespace App;\nclass Kernel {}\n")
	mustWriteFile(t, filepath.Join(root, "src", "Command", "AttributeCommand.php"), `<?php
namespace App\Command;
use Symfony\Component\Console\Attribute\AsCommand;
#[AsCommand('app:attribute')]
class AttributeCommand {}
`)
	mustWriteFile(t, filepath.Join(root, "src", "Command", "LegacyCommand.php"), `<?php
namespace App\Command;
class LegacyCommand {
    protected static $defaultName = 'app:legacy';
}
`)
	for _, tt := range []struct {
		grammar token.Version
		want    string
	}{
		{token.PHP74, "app:legacy"},
		{token.PHP80, "app:attribute app:legacy"},
	} {
		idx := symbols.NewIndexFor(phpparser.OnlyGrammar(tt.grammar))
		idx.IndexFile("file://"+filepath.ToSlash(kernel), "<?php\nnamespace App;\nclass Kernel {}\n")
		names := DiscoverCommands(idx).Names()
		if got := strings.Join(names, " "); got != tt.want {
			t.Errorf("PHP %s: commands %q, want %q", tt.grammar, got, tt.want)
		}
	}
}
