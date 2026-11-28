package laravel

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

const controllerSrc = `<?php

namespace App\Http\Controllers;

use App\Models\User;
use Illuminate\Support\Facades\View;

class UserController
{
    public function show(User $user)
    {
        $posts = $user->posts;
        return view('users.show', ['user' => $user, 'count' => 3])->with('title', 'Profile')->withPosts($posts);
    }

    public function index()
    {
        $users = User::all();
        $filter = 'all';
        return view('users.index', compact('users', 'filter'));
    }

    public function edit(User $user)
    {
        $data = ['user' => $user];
        $data['roles'] = [];
        return View::make('users.edit', $data);
    }

    public function export(Request $request)
    {
        return response()->view('users.export', $request->all());
    }
}
`

func describe(vars []ViewVar) string {
	var parts []string
	for _, v := range vars {
		parts = append(parts, fmt.Sprintf("%s=%s@%d:%d", v.Name, v.Expr, v.Line, v.Col))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func TestViewDataFromPHP(t *testing.T) {
	d := NewViewData("/app")
	d.IndexFile("file:///app/app/Http/Controllers/UserController.php", controllerSrc)

	cases := map[string]string{
		"users.show":  "count=3@12:53 posts=$posts@12:98 title='Profile'@12:74 user=$user@12:36",
		"users.index": "filter=$filter@19:53 users=$users@19:44",
		"users.edit":  "roles=[]@25:15 user=$user@24:18",
	}
	for view, want := range cases {
		in := d.Inputs(view)
		if got := describe(in.Vars); got != want || !in.Complete {
			t.Errorf("%s: got %q complete=%v\nwant %q", view, got, in.Complete, want)
		}
		if len(in.Classes) != 1 || in.Classes[0] != `App\Http\Controllers\UserController` {
			t.Errorf("%s: rendering class %v", view, in.Classes)
		}
	}
	if d.Inputs("users.export").Complete {
		t.Error("data from $request->all() can't be read")
	}

	// Indexing the file again replaces what it said.
	d.IndexFile("file:///app/app/Http/Controllers/UserController.php", "<?php return view('users.show');")
	if in := d.Inputs("users.show"); len(in.Vars) != 0 || !d.RenderedFrom("users.show") {
		t.Errorf("after reindexing: %v", in.Vars)
	}
	if d.RenderedFrom("users.index") {
		t.Error("users.index is no longer rendered")
	}
}

func TestViewDataCompleteness(t *testing.T) {
	d := NewViewData("/app")
	d.IndexFile("file:///app/app/A.php", `<?php return view('pages.a', ['x' => 1]);`)
	d.IndexFile("file:///app/app/B.php", `<?php return view('pages.b', ['x' => 1]);`)
	d.IndexFile("file:///app/routes/web.php", `<?php Route::get('/b', fn () => 'pages.b');`)
	if !d.Inputs("pages.a").Complete {
		t.Error("pages.a has one known render")
	}
	if d.Inputs("pages.b").Complete {
		t.Error("pages.b is named somewhere else, which may render it")
	}
	if d.Inputs("components.alert").Complete || d.Inputs("never.rendered").Complete {
		t.Error("component views and views nothing renders aren't complete")
	}

	// A view rendered by a name built at runtime may be any view.
	d.IndexFile("file:///app/app/C.php", `<?php return view($name);`)
	if d.Inputs("pages.a").Complete {
		t.Error("a dynamic render makes every view incomplete")
	}
	d.IndexFile("file:///app/app/C.php", `<?php`)
	if !d.Inputs("pages.a").Complete {
		t.Error("removing the dynamic render restores completeness")
	}
	d.IndexFile("file:///app/resources/views/x.blade.php", `@include($partial)`)
	if d.Inputs("pages.a").Complete {
		t.Error("a dynamic @include makes every view incomplete")
	}
}

func TestViewDataSharedComposersAndMail(t *testing.T) {
	d := NewViewData("/app")
	d.IndexFile("file:///app/app/Providers/AppServiceProvider.php", `<?php
namespace App\Providers;

use App\View\Composers\NavComposer;
use Illuminate\Support\Facades\View;

class AppServiceProvider
{
    public function boot()
    {
        View::share('site', config('app.name'));
        View::composer('layouts.app', NavComposer::class);
        View::composer(['pages.a', 'pages.b'], function ($view) {
            $view->with('menu', []);
        });
    }
}
`)
	d.IndexFile("file:///app/app/View/Composers/NavComposer.php", `<?php
namespace App\View\Composers;

class NavComposer
{
    public function compose(View $view): void
    {
        $view->with('links', $this->links())->with(['active' => 'home']);
    }
}
`)
	d.IndexFile("file:///app/app/Mail/Welcome.php", `<?php
namespace App\Mail;

class Welcome extends Mailable
{
    public function content(): Content
    {
        return new Content(view: 'mail.welcome', with: ['name' => $this->user->name]);
    }
}
`)
	d.IndexFile("file:///app/app/Http/Controllers/PageController.php", `<?php return view('layouts.app');`)

	got := map[string]string{}
	for _, view := range []string{"layouts.app", "pages.a", "mail.welcome"} {
		got[view] = names(d.Inputs(view).Vars)
	}
	want := map[string]string{
		"layouts.app":  "active links site",
		"pages.a":      "menu site",
		"mail.welcome": "name site",
	}
	for view, w := range want {
		if got[view] != w {
			t.Errorf("%s: got %q, want %q", view, got[view], w)
		}
	}
	if in := d.Inputs("mail.welcome"); len(in.Classes) != 1 || in.Classes[0] != `App\Mail\Welcome` {
		t.Errorf("the Mailable renders mail.welcome: %v", in.Classes)
	}
	if !d.Inputs("layouts.app").Complete {
		t.Error("layouts.app's composer and shared data can all be read")
	}
}

func names(vars []ViewVar) string {
	seen := map[string]bool{}
	var out []string
	for _, v := range vars {
		if !seen[v.Name] {
			seen[v.Name] = true
			out = append(out, v.Name)
		}
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

func TestViewDataThroughTemplates(t *testing.T) {
	d := NewViewData("/app")
	d.IndexFile("file:///app/app/Http/Controllers/PageController.php", `<?php return view('pages.home', ['user' => $u]);`)
	d.IndexFile("file:///app/resources/views/pages/home.blade.php",
		"@extends('layouts.app')\n@include('partials.card', ['title' => $user->name])\n@each('partials.row', $items, 'row')")

	if got := describe(d.Inputs("layouts.app").Vars); got != "user=$u@0:34" {
		t.Errorf("layout: %q", got)
	}
	if got := describe(d.Inputs("partials.card").Vars); got != "title=$user->name@1:28 user=$u@0:34" {
		t.Errorf("partial: %q", got)
	}
	if got := describe(d.Inputs("partials.row").Vars); got != "key=@2:31 row=@2:31" {
		t.Errorf("@each view gets only its item: %q", got)
	}
	if !d.Inputs("partials.card").Complete {
		t.Error("partials.card is included by a view whose renders are all known")
	}
	if d.ViewName("file:///app/resources/views/pages/home.blade.php") != "pages.home" {
		t.Error("ViewName")
	}
}

func TestViewDataKeepsTheNamespaceAcrossNamespaceCalls(t *testing.T) {
	for _, call := range []string{"Route::namespace('Admin')->group(fn () => 1);", "$router->namespace('x');", `namespace\helper();`} {
		d := NewViewData("/app")
		d.IndexFile("file:///app/app/Mail/Welcome.php", `<?php
namespace App\Mail;

`+call+`

class Welcome extends Mailable
{
    public function content(): Content
    {
        return new Content(view: 'mail.welcome', with: ['name' => 'x']);
    }
}
`)
		if in := d.Inputs("mail.welcome"); len(in.Classes) != 1 || in.Classes[0] != `App\Mail\Welcome` {
			t.Errorf("after %s: classes rendering mail.welcome = %v, want App\\Mail\\Welcome", call, in.Classes)
		}
	}
}
