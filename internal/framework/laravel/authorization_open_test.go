package laravel

import (
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/composer"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A file that registers names in a way the reader can't follow leaves them
// open: no name of that kind is reported as unknown then.
func TestRegistrationsTheReaderCantFollowLeaveNamesOpen(t *testing.T) {
	grammars := phpparser.OnlyGrammar(token.Latest)
	uses := "<?php\nuse Illuminate\\Support\\Facades\\Gate;\n"
	for _, c := range []struct {
		name, rel, src string
		want           authOpen
	}{
		{"computed define in a provider", "app/Providers/A.php", "<?php\n$gate->define($name, fn () => true);", authOpen{abilities: true}},
		{"computed define elsewhere", "app/Support/A.php", "<?php\n$gate->define($name, fn () => true);", authOpen{}},
		{"policy guesser", "app/Support/A.php", "<?php\n$gate->guessPolicyNamesUsing(fn () => null);", authOpen{abilities: true}},
		{"instance policy in a provider", "app/Providers/A.php", "<?php\n$gate->policy(Post::class, PostPolicy::class);", authOpen{abilities: true}},
		{"Gate hook", "app/Support/A.php", uses + "Gate::before(fn () => true);", authOpen{abilities: true}},
		{"computed Gate method", "app/Support/A.php", uses + "Gate::{$m}('x');", authOpen{abilities: true}},
		{"computed Gate define", "app/Support/A.php", uses + "Gate::define($name, fn () => true);", authOpen{abilities: true}},
		{"resource without a policy", "app/Support/A.php", uses + "Gate::resource('posts', $policy);", authOpen{abilities: true}},
		{"resource with computed abilities", "app/Support/A.php", uses + "Gate::resource('posts', 'App\\\\P', $map);", authOpen{abilities: true}},
		{"resource with a computed ability", "app/Support/A.php", uses + "Gate::resource('posts', 'App\\\\P', ['view' => $m]);", authOpen{abilities: true}},
		{"computed policy", "app/Support/A.php", uses + "Gate::policy($model, 'App\\\\P');", authOpen{abilities: true}},
		{"computed middleware aliases", "bootstrap/app.php", "<?php\n$app->withMiddleware(function ($m) { $m->alias($aliases); });", authOpen{middleware: true}},
		{"middleware aliases by name", "bootstrap/app.php", "<?php\n$app->withMiddleware(function ($m) { $m->alias(aliases: ['a' => X::class]); });", authOpen{middleware: true}},
		{"computed middleware alias", "bootstrap/app.php", "<?php\n$app->withMiddleware(function ($m) { $m->alias([$name => X::class]); });", authOpen{middleware: true}},
		{"computed middleware group", "bootstrap/app.php", "<?php\n$app->withMiddleware(function ($m) { $m->group($name, []); });", authOpen{middleware: true}},
		{"package provider in config", "config/app.php", "<?php\nreturn ['name' => Acme\\Other::class, 'providers' => [App\\Providers\\A::class, Illuminate\\Auth\\AuthServiceProvider::class, Acme\\Pkg\\Provider::class]];", authOpen{abilities: true, middleware: true}},
		{"own providers in config", "config/app.php", "<?php\nreturn ['name' => Acme\\Other::class, 'providers' => [App\\Providers\\A::class]];", authOpen{}},
		{"package provider in bootstrap", "bootstrap/providers.php", "<?php\nreturn [App\\Providers\\A::class, Acme\\Pkg\\Provider::class];", authOpen{abilities: true, middleware: true}},
		{"own providers in bootstrap", "bootstrap/providers.php", "<?php\nreturn [App\\Providers\\A::class, 'x'];", authOpen{}},
	} {
		if got := readAuthFile("file:///"+c.rel, c.src, c.rel, grammars, []string{`App\`}).open; got != c.want {
			t.Errorf("%s: open = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestPackageProvidersThatRegisterNames(t *testing.T) {
	grammars := phpparser.OnlyGrammar(token.Latest)
	for src, want := range map[string]authOpen{
		"<?php\nGate::define('a', fn () => true);":          {abilities: true},
		"<?php\n$router->aliasMiddleware('a', A::class);":   {middleware: true},
		"<?php\nRoute::middlewareGroup('g', []);":           {middleware: true},
		"<?php\n$gate->policy(A::class, B::class);":         {abilities: true},
		"<?php\n$this->app->singleton(A::class, B::class);": {},
		"<?php\nStr::after('a', 'b');":                      {},
	} {
		if got := readPackageProvider(src, grammars); got != want {
			t.Errorf("readPackageProvider(%q) = %+v, want %+v", src, got, want)
		}
	}

	root := t.TempDir()
	if got := readPackageProviders(root, nil, grammars); got != (authOpen{abilities: true, middleware: true}) {
		t.Errorf("without installed packages everything is open: %+v", got)
	}
	writeFile(t, filepath.Join(root, "composer.json"), `{"autoload": {"psr-4": {"Acme\\": "acme/"}}}`)
	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"), `{"packages": [{"name": "acme/pkg", "extra": {"laravel": {"providers": ["Acme\\Provider"]}}}]}`)
	writeFile(t, filepath.Join(root, "acme", "Provider.php"), "<?php\nnamespace Acme;\nuse Illuminate\\Support\\Facades\\Gate;\nclass Provider { public function boot() { Gate::define('acme', fn () => true); } }\n")
	if got := readPackageProviders(root, composer.GetAutoloadPaths(root), grammars); got != (authOpen{abilities: true}) {
		t.Errorf("a package provider that defines abilities opens them: %+v", got)
	}
	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"), `{"packages": [{"name": "acme/pkg", "extra": {"laravel": {"providers": ["Acme\\Missing"]}}}]}`)
	if got := readPackageProviders(root, composer.GetAutoloadPaths(root), grammars); got != (authOpen{abilities: true, middleware: true}) {
		t.Errorf("a provider that can't be read opens everything: %+v", got)
	}
}

func closedAuthProject(t *testing.T) (*Authorization, *symbols.Index, string) {
	t.Helper()
	auth, idx, root := authProject(t)
	idx.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Auth", "OrderRules.php")), "<?php\nnamespace App\\Auth;\nclass OrderRules {}\n")
	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"), `{"packages": []}`)
	auth.ComposerChanged(composer.GetAutoloadPaths(root))
	return auth, idx, root
}

func TestUnknownAuthNames(t *testing.T) {
	auth, _, _ := closedAuthProject(t)
	for _, c := range []struct {
		name  string
		model ModelArg
		want  bool
	}{
		{"no-such-ability", ModelArg{}, true},
		{"update", ModelArg{}, false},
		{"", ModelArg{}, false},
		{"no-such-ability", ModelArg{Given: true}, false},
		{"update", ModelArg{Given: true, Class: `App\Models\Post`}, false},
		{"no-such-ability", ModelArg{Given: true, Class: `App\Models\Post`}, true},
	} {
		if got := auth.UnknownAbility(c.name, c.model); got != c.want {
			t.Errorf("UnknownAbility(%q, %+v) = %v, want %v", c.name, c.model, got, c.want)
		}
	}
	for name, want := range map[string]bool{"no-such-middleware": true, "auth": false, `App\Http\Middleware\Owner`: false, "": false} {
		if got := auth.UnknownMiddleware(name); got != want {
			t.Errorf("UnknownMiddleware(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestPoliciesThatCantBeReadLeaveAbilitiesOpen(t *testing.T) {
	for name, src := range map[string]string{
		"parent": "<?php\nnamespace App\\Policies;\nclass OrderPolicy extends \\Acme\\BasePolicy {}\n",
		"trait":  "<?php\nnamespace App\\Policies;\nclass OrderPolicy { use \\Acme\\Abilities; }\n",
	} {
		auth, idx, root := closedAuthProject(t)
		idx.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Policies", "OrderPolicy.php")), src)
		if auth.UnknownAbility("no-such-ability", ModelArg{}) {
			t.Errorf("a policy with an unread %s may define any ability", name)
		}
	}
}

func TestAProjectWithoutLaravelLeavesNamesOpen(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"), `{"packages": []}`)
	idx := symbols.NewIndex()
	auth := NewAuthorization(root, idx, func() phpparser.GrammarRange { return phpparser.OnlyGrammar(token.Latest) })
	auth.ScanWorkspace(nil, nil)
	auth.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Providers", "A.php")), "<?php\nuse Illuminate\\Support\\Facades\\Gate;\nGate::resource('posts', 'App\\\\Policies\\\\PostPolicy');\n")
	if auth.UnknownMiddleware("no-such-middleware") {
		t.Error("without Laravel's defaults or a kernel any middleware may exist")
	}
	if auth.UnknownAbility("no-such-ability", ModelArg{}) {
		t.Error("a resource without Laravel's default abilities may define any ability")
	}
}
