package completion

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/composer"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const authTestProvider = `<?php
namespace App\Providers;

use Illuminate\Support\Facades\Gate;

class AuthServiceProvider
{
    public function boot(): void
    {
        Gate::define('view-dashboard', fn ($user) => true);
    }
}
`

const authTestPolicy = `<?php
namespace App\Policies;

class ProductPolicy
{
    public function before($user, $ability) {}
    public function restock($user, $product) {}
}
`

func setupLaravelAuthCompletion(t testing.TB) *Provider {
	t.Helper()
	idx, ca := setupLaravelIndex(t)
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "laravel"))
	if err != nil {
		t.Fatal(err)
	}
	idx.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Policies", "ProductPolicy.php")), authTestPolicy)
	auth := frameworklaravel.NewAuthorization(root, idx, func() phpparser.GrammarRange { return phpparser.OnlyGrammar(token.Latest) })
	auth.ScanWorkspace(composer.GetAutoloadPaths(root), nil)
	auth.IndexFile(symbols.PathToURI(filepath.Join(root, "app", "Providers", "AuthServiceProvider.php")), authTestProvider)
	p := NewProvider(idx, ca, "laravel")
	p.SetLaravelAuthorization(auth)
	return p
}

const authUses = `<?php
use App\Models\User;
use Illuminate\Support\Facades\Gate;
use Illuminate\Support\Facades\Route;
use Illuminate\Routing\Controllers\Middleware;
`

func TestLaravelAbilityCompletion(t *testing.T) {
	p := setupLaravelAuthCompletion(t)
	tests := []struct{ name, src string }{
		{"Gate facade", authUses + "Gate::allows('‸');"},
		{"Gate facade denies", authUses + "Gate::denies('vi‸');"},
		{"Gate any with an array", authUses + "Gate::any(['view-dashboard', '‸']);"},
		{"Gate contract", authUses + "app(\\Illuminate\\Contracts\\Auth\\Access\\Gate::class)->check('‸');"},
		{"user can", authUses + "function f(User $user) { $user->can('‸'); }"},
		{"user cannot", authUses + "function f(User $user) { $user->cannot('‸'); }"},
		{"controller authorize", authUses + "class C { use \\Illuminate\\Foundation\\Auth\\Access\\AuthorizesRequests; function f() { $this->authorize('‸'); } }"},
		{"route can", authUses + "Route::get('/', fn () => 1)->can('‸');"},
		{"route group can", authUses + "Route::can('‸')->group(fn () => 1);"},
		{"can middleware", authUses + "Route::get('/', fn () => 1)->middleware('can:‸');"},
		{"can middleware with a model", authUses + "Route::get('/', fn () => 1)->middleware('can:vi‸,post');"},
		{"empty argument", authUses + "Gate::allows(‸);"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.index.IndexFile("file:///test.php", strings.Replace(tt.src, "‸", "", 1))
			labels := completeMarked(p, tt.src)
			if !labels["view-dashboard"] {
				t.Errorf("Gate::define ability is offered: %v", labels)
			}
			if strings.Contains(tt.src, "vi‸") {
				return
			}
			if !labels["restock"] {
				t.Errorf("policy ability is offered: %v", labels)
			}
			if labels["before"] || labels["auth"] {
				t.Errorf("only abilities are offered: %v", labels)
			}
		})
	}
}

func TestLaravelMiddlewareCompletion(t *testing.T) {
	p := setupLaravelAuthCompletion(t)
	tests := []struct{ name, src string }{
		{"route middleware", authUses + "Route::get('/', fn () => 1)->middleware('‸');"},
		{"route facade middleware array", authUses + "Route::middleware(['auth', '‸'])->group(fn () => 1);"},
		{"without middleware", authUses + "Route::get('/', fn () => 1)->withoutMiddleware('‸');"},
		{"controller middleware attribute", authUses + "new Middleware('‸');"},
		{"controller middleware", authUses + "class C extends \\Illuminate\\Routing\\Controller { function __construct() { $this->middleware('‸'); } }"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.index.IndexFile("file:///test.php", strings.Replace(tt.src, "‸", "", 1))
			labels := completeMarked(p, tt.src)
			for _, want := range []string{"auth", "can", "throttle", "verified", "web", "api"} {
				if !labels[want] {
					t.Errorf("Laravel's %q is offered: %v", want, labels)
				}
			}
			if labels["view-dashboard"] {
				t.Errorf("abilities are not middleware: %v", labels)
			}
		})
	}
}

func TestLaravelAuthNameCompletionEdits(t *testing.T) {
	p := setupLaravelAuthCompletion(t)
	items := completeMarkedItems(p, authUses+"Route::middleware('auth.b‸');")
	var basic *protocol.CompletionItem
	for i := range items {
		if items[i].Label == "auth.basic" {
			basic = &items[i]
		}
	}
	if basic == nil || basic.TextEdit == nil {
		t.Fatalf("auth.basic is offered with an edit: %+v", items)
	}
	if basic.TextEdit.NewText != "auth.basic" || basic.TextEdit.Range.Start.Character != 19 || basic.TextEdit.Range.End.Character != 25 {
		t.Errorf("the edit replaces what is typed inside the quotes: %+v", basic.TextEdit)
	}
	if len(items) != 1 {
		t.Errorf("only names starting with the typed text: %v", collectLabels(items))
	}

	for _, item := range completeMarkedItems(p, authUses+"Gate::allows(‸);") {
		if item.TextEdit == nil || item.TextEdit.NewText != "'"+item.Label+"'" {
			t.Errorf("an empty argument gets a quoted name: %+v", item.TextEdit)
		}
	}
}

func TestLaravelAuthNamesStayOutOfOtherCalls(t *testing.T) {
	p := setupLaravelAuthCompletion(t)
	for _, src := range []string{
		authUses + "class Box { function can($x) {} } (new Box)->can('‸');",
		authUses + "Route::get('/', fn () => 1)->middleware('throttle:6‸');",
		authUses + "Route::get('/', fn () => 1)->middleware('can:update,‸');",
		authUses + "Gate::allows('x', '‸');",
		authUses + "Gate::define('‸');",
	} {
		labels := completeMarked(p, src)
		if labels["view-dashboard"] || labels["auth"] {
			t.Errorf("%s offers authorization names: %v", src[len(authUses):], labels)
		}
	}
}

func BenchmarkLaravelAbilityCompletion(b *testing.B) {
	p := setupLaravelAuthCompletion(b)
	src := authUses + "function f(User $user) { $user->can('re'); }"
	pos := protocol.Position{Line: 5, Character: strings.Index(strings.Split(src, "\n")[5], "re'") + 2}
	b.ReportAllocs()
	for b.Loop() {
		if len(p.GetCompletions("file:///test.php", src, pos)) == 0 {
			b.Fatal("no completion")
		}
	}
}

func TestProviderReadsAuthNamesForTheCheck(t *testing.T) {
	p := setupLaravelAuthCompletion(t)
	src := authUses + "Gate::allows('view-dashboard', Post::class);"
	p.index.IndexFile("file:///test.php", src)
	file := parser.ParseFile(src, token.Latest)
	at := uint32(strings.Index(src, "view-dashboard"))
	arg, ok := ast.CallArgAt(file.Tree, src, at)
	if !ok {
		t.Fatal("no call argument")
	}
	name, ok := p.AuthName(file, arg)
	if !ok || name.Kind != frameworklaravel.AbilityName || name.Name != "view-dashboard" {
		t.Errorf("AuthName = %+v, %v", name, ok)
	}
	if got := p.ModelClass(file, name.Model); got != "Post" && !strings.HasSuffix(got, `\Post`) {
		t.Errorf("ModelClass = %q, want Post", got)
	}
}
