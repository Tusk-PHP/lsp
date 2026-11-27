package laravel

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/composer"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const authProvider = `<?php
namespace App\Providers;

use App\Models\Invoice;
use App\Auth\InvoiceRules;
use App\Policies\PostPolicy;
use Illuminate\Support\Facades\Gate;
use Illuminate\Support\ServiceProvider;

class AuthServiceProvider extends ServiceProvider
{
    protected $policies = [
        \App\Models\Order::class => \App\Auth\OrderRules::class,
    ];

    public function boot(): void
    {
        Gate::define('view-dashboard', fn ($user) => true);
        Gate::define("edit-settings", [PostPolicy::class, 'update']);
        Gate::resource('posts', PostPolicy::class);
        Gate::resource('drafts', PostPolicy::class, ['publish' => 'update']);
        Gate::policy(Invoice::class, InvoiceRules::class);
        Other::define('not-an-ability', fn () => true);
    }
}
`

const authBootstrap = `<?php
use Illuminate\Foundation\Application;
use Illuminate\Foundation\Configuration\Middleware;

return Application::configure(basePath: dirname(__DIR__))
    ->withMiddleware(function (Middleware $middleware): void {
        $middleware->alias(['admin' => \App\Http\Middleware\EnsureAdmin::class]);
        $middleware->appendToGroup('reports', [\App\Http\Middleware\Audit::class]);
    })
    ->create();
`

const authKernel = `<?php
namespace App\Http;

class Kernel
{
    protected $middlewareAliases = [
        'tenant' => Middleware\IdentifyTenant::class,
    ];

    protected $routeMiddleware = [
        'legacy' => Middleware\Legacy::class,
    ];

    protected $middlewareGroups = [
        'console' => [],
    ];
}
`

func authProject(t *testing.T) (*Authorization, *symbols.Index, string) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "composer.json"), `{"autoload": {"psr-4": {"App\\": "app/"}}}`)
	writeFile(t, filepath.Join(root, "app", "Providers", "AuthServiceProvider.php"), authProvider)
	writeFile(t, filepath.Join(root, "bootstrap", "app.php"), authBootstrap)
	writeFile(t, filepath.Join(root, "app", "Http", "Kernel.php"), authKernel)
	vendor := filepath.Join("..", "..", "..", "testdata", "laravel", "vendor", "laravel", "framework", "src", "Illuminate")
	for _, rel := range []string{filepath.Join("Foundation", "Configuration", "Middleware.php"), filepath.Join("Auth", "Access", "Gate.php")} {
		data, err := os.ReadFile(filepath.Join(vendor, rel))
		if err != nil {
			t.Skipf("testdata/laravel vendor not installed: %v", err)
		}
		writeFile(t, filepath.Join(root, "vendor", "laravel", "framework", "src", "Illuminate", rel), string(data))
	}

	idx := symbols.NewIndex()
	idx.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Policies", "PostPolicy.php")), `<?php
namespace App\Policies;

class PostPolicy
{
    public function before($user, $ability) {}
    public function viewAny($user) {}
    public function update($user, $post) {}
    public function after($user, $ability, $result) {}
    public static function make() {}
    protected function owns($user, $post) {}
    public function __construct() {}
}
`)
	idx.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Policies", "CommentPolicy.php")), `<?php
namespace App\Policies;

class CommentPolicy
{
    public function update($user, $comment) {}
}
`)
	idx.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Policies", "Helpers.php")), `<?php
namespace App\Policies;

class Helpers
{
    public function notAnAbility() {}
}
`)
	idx.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Auth", "InvoiceRules.php")), `<?php
namespace App\Auth;

class InvoiceRules
{
    public function refund($user, $invoice) {}
}
`)
	auth := NewAuthorization(root, idx, func() phpparser.GrammarRange { return phpparser.OnlyGrammar(token.Latest) })
	auth.ScanWorkspace(composer.GetAutoloadPaths(root))
	return auth, idx, root
}

func TestAuthorizationAbilitiesFromGateAndPolicies(t *testing.T) {
	auth, _, root := authProject(t)
	names := auth.AbilityNames()
	for _, want := range []string{
		"view-dashboard", "edit-settings",
		"posts.viewAny", "posts.view", "posts.create", "posts.update", "posts.delete",
		"drafts.publish",
		"viewAny", "update",
		"refund",
	} {
		if !slices.Contains(names, want) {
			t.Errorf("ability %q is known: %v", want, names)
		}
	}
	for _, unwanted := range []string{"before", "after", "make", "owns", "__construct", "notAnAbility", "not-an-ability", "drafts.update"} {
		if slices.Contains(names, unwanted) {
			t.Errorf("%q is no ability: %v", unwanted, names)
		}
	}
	if !slices.IsSorted(names) {
		t.Errorf("ability names are sorted: %v", names)
	}

	define := auth.Abilities("view-dashboard")
	providerURI := symbols.PathToURI(filepath.Join(root, "app", "Providers", "AuthServiceProvider.php"))
	if len(define) != 1 || define[0].URI != providerURI || define[0].Range.Start.Line != 17 || define[0].Range.Start.Character != 22 {
		t.Errorf("Gate::define is found at its name: %+v", define)
	}
	resource := auth.Abilities("drafts.publish")
	if len(resource) != 1 || resource[0].Policy != `App\Policies\PostPolicy` || resource[0].Method != "update" || resource[0].Range.Start.Line != 7 {
		t.Errorf("Gate::resource goes to the policy method it maps to: %+v", resource)
	}
	if update := auth.Abilities("update"); len(update) != 2 {
		t.Errorf("update is answered by both discovered policies: %+v", update)
	}
}

func TestAuthorizationAbilityForModel(t *testing.T) {
	auth, idx, root := authProject(t)
	idx.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Models", "Comment.php")), "<?php\nnamespace App\\Models;\nclass Comment {}\n")
	tests := []struct{ model, policy string }{
		{`App\Models\Comment`, `App\Policies\CommentPolicy`},
		{`App\Models\Post`, `App\Policies\PostPolicy`},
		{`\App\Models\Comment`, `App\Policies\CommentPolicy`},
		{"", `App\Policies\CommentPolicy`},
	}
	for _, tt := range tests {
		ability, ok := auth.AbilityFor("update", tt.model)
		if !ok || ability.Policy != tt.policy {
			t.Errorf("update for %q is %+v, want %s", tt.model, ability, tt.policy)
		}
	}
	if ability, ok := auth.AbilityFor("refund", `App\Models\Invoice`); !ok || ability.Policy != `App\Auth\InvoiceRules` {
		t.Errorf("a policy registered with Gate::policy answers for its model: %+v", ability)
	}
	if _, ok := auth.AbilityFor("missing", ""); ok {
		t.Error("an unknown ability has no definition")
	}
}

func TestGuessPolicyNamesFollowsLaravel(t *testing.T) {
	got := guessPolicyNames(`App\Models\Billing\Invoice`)
	want := []string{
		`App\Models\Policies\Billing\InvoicePolicy`,
		`App\Policies\Billing\InvoicePolicy`,
		`App\Models\Billing\Policies\InvoicePolicy`,
		`App\Models\Policies\InvoicePolicy`,
		`App\Policies\InvoicePolicy`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("guessPolicyNames = %v, want %v", got, want)
	}
	if got := guessPolicyNames("Invoice"); got != nil {
		t.Errorf("a class outside a namespace has no policy names: %v", got)
	}
}

func TestAuthorizationMiddlewareFromAppAndFramework(t *testing.T) {
	auth, idx, root := authProject(t)
	names := auth.MiddlewareNames()
	for _, want := range []string{"admin", "reports", "tenant", "legacy", "console", "auth", "can", "throttle", "verified", "web", "api"} {
		if !slices.Contains(names, want) {
			t.Errorf("middleware %q is known: %v", want, names)
		}
	}
	admin := auth.Middleware("admin")
	if len(admin) != 1 || admin[0].Kind != MiddlewareAlias || admin[0].Class != `App\Http\Middleware\EnsureAdmin` {
		t.Errorf("a bootstrap/app.php alias names its class: %+v", admin)
	}
	if tenant := auth.Middleware("tenant"); len(tenant) != 1 || tenant[0].Class != `App\Http\Middleware\IdentifyTenant` {
		t.Errorf("a kernel alias resolves its class in the kernel's namespace: %+v", tenant)
	}
	if web := auth.Middleware("web"); len(web) != 1 || web[0].Kind != MiddlewareGroup {
		t.Errorf("web is Laravel's group: %+v", web)
	}
	if authAlias := auth.Middleware("auth"); len(authAlias) != 1 || authAlias[0].Class != `Illuminate\Auth\Middleware\Authenticate` {
		t.Errorf("auth is Laravel's alias of Authenticate: %+v", authAlias)
	}

	bootstrap := symbols.PathToURI(filepath.Join(root, "bootstrap", "app.php"))
	auth.IndexFile(bootstrap, "<?php\nreturn Application::configure()->withMiddleware(function ($m) {\n    $m->alias(['owner' => Owner::class]);\n})->create();\n")
	if names := auth.MiddlewareNames(); slices.Contains(names, "admin") || !slices.Contains(names, "owner") {
		t.Errorf("an edited bootstrap/app.php replaces its aliases: %v", names)
	}
	auth.IndexFile(bootstrap, "")
	if slices.Contains(auth.MiddlewareNames(), "owner") {
		t.Error("a deleted bootstrap/app.php registers nothing")
	}

	idx.RemoveFile(symbols.PathToURI(filepath.Join(root, "app", "Policies", "CommentPolicy.php")))
	if update := auth.Abilities("update"); len(update) != 1 {
		t.Errorf("a removed policy no longer answers: %+v", update)
	}
	auth.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Services", "Gates.php")), "<?php\nGate::define('ignored', fn () => true);\n")
	if slices.Contains(auth.AbilityNames(), "ignored") {
		t.Error("only providers, bootstrap/app.php and the HTTP kernel are read")
	}
}
