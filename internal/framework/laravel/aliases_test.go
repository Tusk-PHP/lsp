package laravel

import (
	"os"
	"path/filepath"
	"testing"

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
	aliases := NewAliases(root)
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
`)
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

func TestConfigAliasesUnterminated(t *testing.T) {
	got := configAliases("<?php\nreturn ['aliases' => ['Str' => 'Illuminate\\Support\\Str'")
	if len(got) != 1 || got["Str"] != `Illuminate\Support\Str` {
		t.Fatalf("configAliases = %v", got)
	}
}

func TestConfigAliasesReadsTheLexer(t *testing.T) {
	got := configAliases(`<?php
return [
    'name' => 'aliases',
    "aliases" => Facade::defaultAliases()->merge([
        'Str' => 'Illuminate\Support\Str',
        'Mix' => 'A' . 'B',
        'Arr' => \Illuminate\Support\Arr::class,
        'Bad' =>
    ])->toArray(),
];
`)
	want := map[string]string{"Str": `Illuminate\Support\Str`, "Arr": `Illuminate\Support\Arr`}
	if len(got) != len(want) || got["Str"] != want["Str"] || got["Arr"] != want["Arr"] {
		t.Fatalf("configAliases = %v, want %v", got, want)
	}
}

func TestConfigAliasesResolveImportedAndNamespacedNames(t *testing.T) {
	got := configAliases(`<?php
namespace Config;

use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\Redis as Store;

return [
    'aliases' => [
        'Cache' => Cache::class,
        'Store' => Store::class,
        'Local' => Local::class,
        'Nameless' => \::class,
    ],
];
`)
	want := map[string]string{
		"Cache": `Illuminate\Support\Facades\Cache`,
		"Store": `Illuminate\Support\Facades\Redis`,
		"Local": `Config\Local`,
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
