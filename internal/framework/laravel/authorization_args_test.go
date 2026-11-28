package laravel

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const authStubs = `<?php
namespace Illuminate\Contracts\Auth\Access { interface Gate {} interface Authorizable {} }
namespace Illuminate\Support\Facades { class Gate {} class Route {} }
namespace Illuminate\Foundation\Auth\Access { trait AuthorizesRequests {} }
namespace Illuminate\Routing { class Route {} class Controller {} }
namespace Illuminate\Routing\Controllers { class Middleware {} }
namespace App\Models { class User implements \Illuminate\Contracts\Auth\Access\Authorizable {} class Post {} }
`

func TestAuthNameArg(t *testing.T) {
	uses := "<?php\nuse App\\Models\\User;\nuse App\\Models\\Post;\nuse Illuminate\\Support\\Facades\\Gate;\nuse Illuminate\\Support\\Facades\\Route;\nuse Illuminate\\Routing\\Controllers\\Middleware;\n" +
		"/** @param User|Post $either */\nfunction f(User $user, Post $post, \\Illuminate\\Contracts\\Auth\\Access\\Gate $gate, $either, \\Illuminate\\Routing\\Route $route, $cls) {\n"
	for _, c := range []struct {
		code          string
		ok            bool
		kind          AuthNameKind
		name, partial string
		model         string
	}{
		{code: "Gate::allows('vi|ew');", ok: true, kind: AbilityName, name: "view", partial: "vi"},
		{code: "$gate->check('ed|it', $post);", ok: true, kind: AbilityName, name: "edit", partial: "ed", model: "$post"},
		{code: "$user->can('up|');", ok: true, kind: AbilityName, name: "up", partial: "up"},
		{code: "$either->can('up|');", ok: true, kind: AbilityName, name: "up", partial: "up"},
		{code: "Route::middleware('au|th');", ok: true, kind: MiddlewareName, name: "auth", partial: "au"},
		{code: "$route->middleware('throttle|:6');", ok: true, kind: MiddlewareName, name: "throttle", partial: "throttle"},
		{code: "$route->middleware('can:up|date,post');", ok: true, kind: AbilityName, name: "update", partial: "up"},
		{code: "$route->middleware('can:update,po|');"},
		{code: "$route->middleware('throttle:6|');"},
		{code: "new Middleware('au|th');", ok: true, kind: MiddlewareName, name: "auth", partial: "au"},
		{code: "Gate::allows(vi|ew);"},
		{code: "Gate::allows('a', 'b|');"},
		{code: "Post::allows('a|');"},
		{code: "$cls::allows('a|');"},
		{code: "$post->can('a|');"},
		{code: "can('a|');"},
		{code: "new Post('a|');"},
	} {
		src := uses + strings.Replace(c.code, "|", "", 1) + "\n}\n"
		off := uint32(len(uses) + strings.Index(c.code, "|"))
		idx := symbols.NewIndex()
		idx.IndexFile("file:///stubs.php", authStubs)
		idx.IndexFile("file:///f.php", src)
		file := parser.ParseFile(src, token.Latest)
		arg, ok := ast.CallArgAt(file.Tree, src, off)
		if !ok {
			t.Fatalf("no call argument in %q", c.code)
		}
		name, ok := AuthNameArg(resolve.NewResolver(idx), idx, file, arg)
		model := ""
		if name.Model != nil {
			model = name.Model.Bounds().Text(src)
		}
		if ok != c.ok || name.Kind != c.kind || name.Name != c.name || name.Partial != c.partial || model != c.model {
			t.Errorf("AuthNameArg(%q) = %+v (model %q), %v", c.code, name, model, ok)
		}
	}
}

func TestAbilityOfPrefersTheCheckedModelsPolicy(t *testing.T) {
	auth, idx, _ := authProject(t)
	src := "<?php\nnamespace App\\Models;\nclass Post {}\nfunction f(Post $post, $x) {\n    g(Post::class, $post, $x::class, Post::LABEL);\n}\n"
	idx.IndexFile("file:///app/Models/Post.php", src)
	file := parser.ParseFile(src, token.Latest)
	var args []*ast.Arg
	ast.Walk(file.Tree, func(n ast.Node) bool {
		if call, ok := n.(*ast.Call); ok {
			args = call.Args.Args
		}
		return true
	})
	r := resolve.NewResolver(idx)
	for i, want := range []string{`App\Policies\PostPolicy`, `App\Policies\PostPolicy`, "", ""} {
		ability, ok := auth.AbilityOf(r, file, AuthName{Kind: AbilityName, Name: "update", Model: args[i].Value})
		if !ok || want != "" && ability.Policy != want {
			t.Errorf("update checked against %s = %+v, %v; want %s", args[i].Value.Bounds().Text(src), ability, ok, want)
		}
	}
	if _, ok := auth.AbilityOf(r, file, AuthName{Kind: AbilityName, Name: "update"}); !ok {
		t.Error("an ability checked without a model is still found")
	}
}
