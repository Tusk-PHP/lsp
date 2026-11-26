package laravel

import (
	"os"
	"path/filepath"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func latestGrammar(string) phpparser.GrammarRange {
	return phpparser.OnlyGrammar(token.Latest)
}

// pinnedToPHP74 indexes source at uri after a first text that only PHP 7.4
// reads cleanly, so the index keeps reading uri as PHP 7.4 while the newest
// grammar of its range reads source cleanly too.
func pinnedToPHP74(t *testing.T, uri, source string) *symbols.Index {
	t.Helper()
	idx := symbols.NewIndexFor(phpparser.GrammarsBetween(token.PHP74, token.Latest))
	idx.IndexFile(uri, source+"match('pin');\n")
	idx.IndexFile(uri, source)
	if tree := idx.GrammarOf(uri).Parse(source); tree.Version != token.PHP74 || !tree.Clean() {
		t.Fatalf("the index reads %s as PHP %s (clean %v), want a clean PHP 7.4 tree", uri, tree.Version, tree.Clean())
	}
	if tree := idx.Grammars().Parse(source); tree.Version != token.Latest || !tree.Clean() {
		t.Fatalf("the whole range reads %s as PHP %s (clean %v), want a clean tree of the newest grammar", uri, tree.Version, tree.Clean())
	}
	return idx
}

// aliasProject writes a Laravel app with a config alias, a discovered package
// alias and a package the app opts out of, and indexes the classes they name
// beside Laravel's route() helper.
func aliasProject(t *testing.T) (*Aliases, *symbols.Index) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "composer.json"), `{"extra": {"laravel": {"dont-discover": ["acme/hidden"]}}}`)
	writeFile(t, filepath.Join(root, "config", "app.php"), `<?php
use Illuminate\Support\Facades\Facade;
use App\Support\Money;

return [
    'name' => env('APP_NAME', 'Laravel'),
    'aliases' => Facade::defaultAliases()->merge([
        'Money' => Money::class,
        'Legacy' => 'App\Support\LegacyHelper',
        'Gone' => App\Support\Missing::class,
    ])->toArray(),
    'locale' => 'en',
];
`)
	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"), `{"packages": [
  {"name": "livewire/flux", "extra": {"laravel": {"aliases": {"Flux": "Flux\\Flux"}}}},
  {"name": "acme/hidden", "extra": {"laravel": {"aliases": {"Hidden": "Acme\\Hidden"}}}},
  {"name": "odd/extra", "extra": {"laravel": {"aliases": ["not", "a", "map"]}}}
]}`)

	idx := symbols.NewIndex()
	idx.IndexFile("file:///Route.php", `<?php
namespace Illuminate\Support\Facades;
/** @method static bool has(string $name) */
class Route extends Facade {}
`)
	idx.IndexFile("file:///helpers.php", `<?php
function route($name) {}
`)
	idx.IndexFile("file:///Money.php", `<?php
namespace App\Support;
class Money {}
class LegacyHelper {}
`)
	idx.IndexFile("file:///Flux.php", `<?php
namespace Flux;
class Flux {}
`)
	idx.IndexFile("file:///Hidden.php", `<?php
namespace Acme;
class Hidden {}
`)
	aliases := NewAliases(root, latestGrammar)
	aliases.Load()
	idx.SetClassGenerator(aliases)
	return aliases, idx
}

func TestAliasesResolveInGlobalNamespace(t *testing.T) {
	_, idx := aliasProject(t)
	for alias, target := range map[string]string{
		"Route":  `Illuminate\Support\Facades\Route`, // Laravel default, beside route()
		"Money":  `App\Support\Money`,                // config/app.php, ::class
		"Legacy": `App\Support\LegacyHelper`,         // config/app.php, string
		"Flux":   `Flux\Flux`,                        // package discovery
	} {
		sym := idx.LookupFold(alias, func(s *symbols.Symbol) bool { return s.Kind == symbols.KindClass })
		if sym == nil {
			t.Errorf("alias %s not resolved", alias)
			continue
		}
		if sym.Extends != target {
			t.Errorf("alias %s extends %q, want %q", alias, sym.Extends, target)
		}
	}
	// Go to definition leads to the class the alias stands for.
	if sym := idx.Lookup("Route"); sym == nil || sym.URI != "file:///Route.php" {
		t.Errorf("Route alias points at %v, want the facade's file", sym)
	}
}

func TestAliasesStayUnknownWhenUnsure(t *testing.T) {
	_, idx := aliasProject(t)
	for _, name := range []string{
		"Hidden",         // package listed in dont-discover
		"Gone",           // target class isn't indexed
		"Cache",          // default alias whose facade isn't indexed
		`App\Http\Route`, // aliases live in the global namespace only
		"NotAnAlias",
	} {
		if sym := idx.Lookup(name); sym != nil {
			t.Errorf("%s resolved to %s, want unknown", name, sym.FQN)
		}
	}
}

func TestConfigAliasesOldStyleList(t *testing.T) {
	got := configAliases(`<?php
return [
    'providers' => [App\Providers\AppServiceProvider::class],
    'aliases' => [
        'App' => Illuminate\Support\Facades\App::class,
        "Redis" => Illuminate\Support\Facades\Redis::class,
    ],
];
`, phpparser.OnlyGrammar(token.Latest))
	want := map[string]string{
		"App":   `Illuminate\Support\Facades\App`,
		"Redis": `Illuminate\Support\Facades\Redis`,
	}
	if len(got) != len(want) {
		t.Fatalf("configAliases = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("configAliases[%s] = %q, want %q", k, got[k], v)
		}
	}
}

func TestAliasesReadConfigWithTheIndexsGrammar(t *testing.T) {
	root := t.TempDir()
	appConfig := filepath.Join(root, "config", "app.php")
	source := `<?php
namespace App\Support;

#[Pinned] function pinned() {} use App\Billing\Money;

return [
    'aliases' => ['Money' => Money::class],
];
`
	writeFile(t, appConfig, source)
	idx := pinnedToPHP74(t, symbols.PathToURI(appConfig), source)
	aliases := NewAliases(root, idx.GrammarOf)
	aliases.Load()

	if got, _ := aliases.Target("Money"); got != `App\Support\Money` {
		t.Errorf("Money stands for %q, want the class PHP 7.4 reads, where the import is a comment", got)
	}
}
