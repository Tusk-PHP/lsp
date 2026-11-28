package laravel

import (
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestReadAuthFileSkipsWhatItCantRead(t *testing.T) {
	src := `<?php
use Illuminate\Support\Facades\Gate;
$gate::define('dynamic-gate', fn () => true);
Gate::define(ability: 'named', callback: fn () => true);
Gate::define();
Gate::resource('no-policy', $policy);
Gate::resource('posts', 'App\\Policies\\PostPolicy', $abilities);
Gate::resource('comments', 'App\\Policies\\CommentPolicy', ['view' => 'show', , 'skip' => $m]);
Gate::policy($model::class, PostPolicy::class);
Gate::policy(class: Post::class, policy: PostPolicy::class);
Route::aliasMiddleware($name, Owner::class);
class Kernel {
    protected $policies = [\App\Models\Post::class => 'App\\Policies\\PostPolicy', , 'NoNamespace' => Other::class];
    protected $middlewareAliases = ['auth', 'owner' => \App\Http\Owner::class, , 'blank' => ''];
    protected $middlewareGroups = null;
    public $routeMiddleware;
}
`
	out := readAuthFile("file:///p.php", src, phpparser.OnlyGrammar(token.Latest))
	if len(out.abilities) != 0 {
		t.Errorf("abilities = %+v, want none", out.abilities)
	}
	if len(out.resources) != 1 || out.resources[0].name != "comments" || len(out.resources[0].abilities) != 1 {
		t.Errorf("resources = %+v, want comments with its view ability", out.resources)
	}
	if len(out.policies) != 1 || out.policies[0].model != `App\Models\Post` || out.policies[0].policy != `App\Policies\PostPolicy` {
		t.Errorf("policies = %+v, want the Post policy", out.policies)
	}
	if len(out.middleware) != 2 || out.middleware[0].Name != "owner" || out.middleware[0].Class != `App\Http\Owner` || out.middleware[1].Name != "blank" || out.middleware[1].Class != "" {
		t.Errorf("middleware = %+v, want owner and blank", out.middleware)
	}
}

func TestReadFrameworkAuthWithoutLaravel(t *testing.T) {
	if got := readFrameworkAuth(t.TempDir(), phpparser.OnlyGrammar(token.Latest)); len(got.middleware) != 0 || len(got.resource) != 0 {
		t.Errorf("readFrameworkAuth without vendor = %+v, want nothing", got)
	}
}

func TestMayRegisterNamesNeedsTheGateName(t *testing.T) {
	if mayRegisterNames("<?php\n$x as $y;\n$gate::define('a');", token.Latest) {
		t.Error("define on a variable class isn't a Gate registration")
	}
}
