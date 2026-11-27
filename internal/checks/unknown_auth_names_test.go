package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const authModels = `<?php
namespace App\Models;
class User { use \Illuminate\Foundation\Auth\Access\Authorizable; }
class Post {}
`

var authVendorClasses = []string{
	"Support/Facades/Facade.php", "Support/Facades/Gate.php", "Support/Facades/Route.php",
	"Routing/Route.php", "Routing/RouteRegistrar.php", "Foundation/Auth/Access/Authorizable.php",
	"Contracts/Auth/Access/Authorizable.php", "Contracts/Auth/Access/Gate.php",
}

const authProviderSrc = `<?php
namespace App\Providers;

use Illuminate\Support\Facades\Gate;

class AuthServiceProvider
{
    public function boot(): void
    {
        Gate::define('view-dashboard', fn ($user) => true);
%s
    }
}
`

type authFixture struct {
	files map[string]string
	drop  []string
}

func authBaseFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{
		"composer.json":                           `{"autoload": {"psr-4": {"App\\": "app/"}}}`,
		"vendor/composer/installed.json":          `{"packages": []}`,
		"app/Providers/AuthServiceProvider.php":   fmt.Sprintf(authProviderSrc, ""),
		"app/Policies/PostPolicy.php":             "<?php\nnamespace App\\Policies;\nclass PostPolicy {\n    public function update($user, $post) {}\n}\n",
		"app/Http/Controllers/HomeController.php": "<?php\nnamespace App\\Http\\Controllers;\nclass HomeController {}\n",
	}
	vendor := filepath.Join("..", "..", "testdata", "laravel", "vendor", "laravel", "framework", "src", "Illuminate")
	for _, rel := range []string{"Foundation/Configuration/Middleware.php", "Auth/Access/Gate.php"} {
		data, err := os.ReadFile(filepath.Join(vendor, filepath.FromSlash(rel)))
		if err != nil {
			t.Skipf("testdata/laravel vendor not installed: %v", err)
		}
		files["vendor/laravel/framework/src/Illuminate/"+rel] = string(data)
	}
	return files
}

func checkAuthNames(t *testing.T, fx authFixture, src string) []Finding {
	t.Helper()
	root := t.TempDir()
	files := authBaseFiles(t)
	for path, text := range fx.files {
		files[path] = text
	}
	for _, path := range fx.drop {
		delete(files, path)
	}
	var project []string
	for rel, text := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(rel, "vendor/") && strings.HasSuffix(rel, ".php") {
			project = append(project, path)
		}
	}
	slices.Sort(project)

	idx := symbols.NewIndex()
	idx.IndexFile("file:///models.php", authModels)
	for _, rel := range authVendorClasses {
		path := filepath.Join("..", "..", "testdata", "laravel", "vendor", "laravel", "framework", "src", "Illuminate", filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("testdata/laravel vendor not installed: %v", err)
		}
		idx.IndexFileWithSource(symbols.PathToURI(path), string(data), symbols.SourceVendor)
	}
	for _, path := range project {
		data, _ := os.ReadFile(path)
		idx.IndexFile(symbols.PathToURI(path), string(data))
	}
	grammars := func() phpparser.GrammarRange { return phpparser.OnlyGrammar(token.Latest) }
	auth := frameworklaravel.NewAuthorization(root, idx, grammars)
	auth.ScanWorkspace(composer.GetAutoloadPaths(root), project)
	p := completion.NewProvider(idx, container.NewContainerAnalyzer(idx, root, "laravel"), "laravel")
	p.SetLaravelAuthorization(auth)

	uri := symbols.PathToURI(filepath.Join(root, "app", "Http", "Controllers", "Check.php"))
	idx.IndexFile(uri, src)
	file := parser.ParseFileIn(src, grammars())
	return (&UnknownAuthNameRule{Authorization: auth, Names: p}).Check(file, src, idx)
}

const authCheckUses = `<?php
use App\Models\Post;
use App\Models\User;
use Illuminate\Support\Facades\Gate;
use Illuminate\Support\Facades\Route;
`

func findingTexts(src string, findings []Finding) []string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, string(f.Code)+" "+lines[f.StartLine][f.StartCol:f.EndCol])
	}
	return out
}

func TestUnknownAuthNamesReported(t *testing.T) {
	src := authCheckUses + `
function f(User $user, $anything) {
    Gate::allows('edti-post');
    $user->can('nope');
    Gate::allows('missing', Post::class);
    Route::get('/', fn () => 1)->middleware('auht');
    Route::get('/', fn () => 1)->middleware(['auth', 'verfied', 'throttle:60,1', 'can:nope-either,post', 'can:typo']);
    Route::middleware('wbe')->group(fn () => 1);
    Route::get('/', fn () => 1)->can('nope-route');
}
`
	got := findingTexts(src, checkAuthNames(t, authFixture{}, src))
	want := []string{
		"unknown-ability edti-post",
		"unknown-ability nope",
		"unknown-ability missing",
		"unknown-middleware auht",
		"unknown-middleware verfied",
		"unknown-ability typo",
		"unknown-middleware wbe",
		"unknown-ability nope-route",
	}
	if !slices.Equal(got, want) {
		t.Errorf("findings = %v\nwant %v", got, want)
	}
}

func TestKnownAuthNamesAreQuiet(t *testing.T) {
	src := authCheckUses + `
function f(User $user, $anything, string $name) {
    Gate::allows('view-dashboard');
    Gate::allows('update', Post::class);
    $user->can('update', $anything);
    Gate::allows('whatever', $anything);
    Gate::allows($name);
    Gate::allows("dyn-{$name}");
    Route::get('/', fn () => 1)->middleware(['web', 'api', 'auth:sanctum', 'throttle:60,1', 'can:update,post', 'can:whatever,post', 'App\Http\Middleware\Custom']);
    (new Box)->can('nope');
    Gate::define('new-one', fn () => true);
}
class Box { public function can($x) {} }
`
	if got := checkAuthNames(t, authFixture{}, src); len(got) != 0 {
		t.Errorf("known or uncheckable names are quiet: %v", findingTexts(src, got))
	}
}

func TestUnknownAuthNamesQuietWhileRegistrationsAreOpen(t *testing.T) {
	src := authCheckUses + `
function f(User $user) {
    Gate::allows('edti-post');
    Route::get('/', fn () => 1)->middleware('auht');
}
`
	const packageProvider = "<?php\nnamespace Acme;\nuse Illuminate\\Support\\Facades\\Gate;\nclass AcmeServiceProvider {\n    public function boot() { %s }\n}\n"
	installed := `{"packages": [{"name": "acme/pkg", "autoload": {"psr-4": {"Acme\\": "src/"}}, "install-path": "../acme/pkg", "extra": {"laravel": {"providers": ["Acme\\AcmeServiceProvider"]}}}]}`
	provider := func(extra string) map[string]string {
		return map[string]string{"app/Providers/AuthServiceProvider.php": fmt.Sprintf(authProviderSrc, extra)}
	}
	tests := []struct {
		name       string
		fx         authFixture
		abilities  bool
		middleware bool
	}{
		{"computed Gate::define name", authFixture{files: provider("Gate::define($name, fn () => true);")}, false, true},
		{"Gate::before", authFixture{files: provider("Gate::before(fn () => null);")}, false, true},
		{"Gate::after", authFixture{files: provider("Gate::after(fn () => null);")}, false, true},
		{"custom policy discovery", authFixture{files: provider("Gate::guessPolicyNamesUsing(fn ($m) => $m);")}, false, true},
		{"computed resource abilities", authFixture{files: provider("Gate::resource('posts', \\App\\Policies\\PostPolicy::class, $map);")}, false, true},
		{"a computed Gate::define outside app/Providers", authFixture{files: map[string]string{"routes/web.php": "<?php\nuse Illuminate\\Support\\Facades\\Gate;\nGate::define($name, fn () => true);\n"}}, false, true},
		{"Gate::before in a test", authFixture{files: map[string]string{"tests/TestCase.php": "<?php\nuse Illuminate\\Support\\Facades\\Gate;\nGate::before(fn () => true);\n"}}, false, true},
		{"custom policy discovery outside the providers", authFixture{files: map[string]string{"app/Support/Policies.php": "<?php\n$gate->guessPolicyNamesUsing(fn ($m) => $m);\n"}}, false, true},
		{"a literal Gate::define elsewhere names another ability", authFixture{files: map[string]string{"routes/web.php": "<?php\nuse Illuminate\\Support\\Facades\\Gate as Authz;\nAuthz::define('other', fn () => true);\n"}}, true, true},
		{"unrelated before() and after() calls elsewhere", authFixture{files: map[string]string{"app/Support/Text.php": "<?php\nuse Illuminate\\Support\\Str;\nStr::after('a', 'b');\n$table->string('x')->after('y');\nRoute::resource('posts', X::class);\n"}}, true, true},
		{"a policy with __call", authFixture{files: map[string]string{"app/Policies/PostPolicy.php": "<?php\nnamespace App\\Policies;\nclass PostPolicy {\n    public function __call($m, $a) {}\n}\n"}}, false, true},
		{"a policy extending a class that isn't indexed", authFixture{files: map[string]string{"app/Policies/PostPolicy.php": "<?php\nnamespace App\\Policies;\nclass PostPolicy extends \\Vendor\\BasePolicy {}\n"}}, false, true},
		{"a package provider defining gates", authFixture{files: map[string]string{
			"vendor/composer/installed.json":              installed,
			"vendor/acme/pkg/src/AcmeServiceProvider.php": fmt.Sprintf(packageProvider, "Gate::define('edti-post', fn () => true);"),
		}}, false, true},
		{"a package provider aliasing middleware", authFixture{files: map[string]string{
			"vendor/composer/installed.json":              installed,
			"vendor/acme/pkg/src/AcmeServiceProvider.php": fmt.Sprintf(packageProvider, "$this->app['router']->aliasMiddleware('auht', X::class);"),
		}}, true, false},
		{"a package provider that can't be read", authFixture{files: map[string]string{"vendor/composer/installed.json": installed}}, false, false},
		{"no installed packages list", authFixture{drop: []string{"vendor/composer/installed.json"}}, false, false},
		{"a vendor provider registered by hand", authFixture{files: map[string]string{"bootstrap/providers.php": "<?php\nreturn [\n    App\\Providers\\AuthServiceProvider::class,\n    Acme\\AcmeServiceProvider::class,\n];\n"}}, false, false},
		{"a computed middleware alias", authFixture{files: map[string]string{"bootstrap/app.php": "<?php\nreturn Application::configure()->withMiddleware(function ($m) { $m->alias($aliases); })->create();\n"}}, true, false},
		{"no Laravel defaults to read", authFixture{drop: []string{"vendor/laravel/framework/src/Illuminate/Foundation/Configuration/Middleware.php"}}, true, false},
		{"a computed middleware alias outside bootstrap/app.php", authFixture{files: map[string]string{"app/Support/Routing.php": "<?php\nRoute::aliasMiddleware($name, X::class);\n"}}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkAuthNames(t, tt.fx, src)
			var abilities, middleware bool
			for _, f := range got {
				abilities = abilities || f.Code == rule.UnknownAbility
				middleware = middleware || f.Code == rule.UnknownMiddleware
			}
			if abilities != tt.abilities || middleware != tt.middleware {
				t.Errorf("abilities reported %v (want %v), middleware reported %v (want %v): %v", abilities, tt.abilities, middleware, tt.middleware, findingTexts(src, got))
			}
		})
	}
}

func TestUnknownAuthNamesWithAKernelInsteadOfDefaults(t *testing.T) {
	src := authCheckUses + "Route::get('/', fn () => 1)->middleware(['admin', 'auth', 'web']);\n"
	fx := authFixture{
		drop: []string{"vendor/laravel/framework/src/Illuminate/Foundation/Configuration/Middleware.php"},
		files: map[string]string{"app/Http/Kernel.php": `<?php
namespace App\Http;
class Kernel {
    protected $middlewareGroups = ['web' => []];
    protected $middlewareAliases = ['admin' => Middleware\Admin::class];
}
`},
	}
	if got := findingTexts(src, checkAuthNames(t, fx, src)); !slices.Equal(got, []string{"unknown-middleware auth"}) {
		t.Errorf("a Laravel 10 kernel lists every alias: %v", got)
	}
}

func TestUnknownAuthNamesTrustRegistrationsAnywhereInTheProject(t *testing.T) {
	src := authCheckUses + `
Gate::allows('edti-post');
Gate::allows('from-a-test');
Gate::allows('still-typo');
Route::get('/', fn () => 1)->middleware(['auht', 'admin-panel', 'still-bad']);
`
	fx := authFixture{files: map[string]string{
		"routes/web.php":             "<?php\nuse Illuminate\\Support\\Facades\\Gate as Authz;\nAuthz::define('edti-post', fn () => true);\n",
		"tests/Feature/GateTest.php": "<?php\nnamespace Tests\\Feature;\nuse Illuminate\\Support\\Facades\\Gate;\nclass GateTest {\n    public function test() { Gate::define('from-a-test', fn () => true); }\n}\n",
		"app/Support/Routing.php":    "<?php\nRoute::aliasMiddleware('auht', X::class);\n$router->middlewareGroup('admin-panel', []);\n",
	}}
	got := findingTexts(src, checkAuthNames(t, fx, src))
	if want := []string{"unknown-ability still-typo", "unknown-middleware still-bad"}; !slices.Equal(got, want) {
		t.Errorf("names registered outside the providers are trusted: %v, want %v", got, want)
	}
}
