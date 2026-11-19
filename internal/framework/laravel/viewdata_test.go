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

    public function plain()
    {
        // view('commented.out', ['x' => 1]);
        return view($this->name, ['a' => 1]);
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
		vars, complete := d.Vars(view)
		if got := describe(vars); got != want || !complete {
			t.Errorf("%s: got %q complete=%v\nwant %q", view, got, complete, want)
		}
	}
	if _, complete := d.Vars("users.export"); complete {
		t.Error("data from $request->all() can't be read")
	}
	if d.RenderedFrom("commented.out") {
		t.Error("a render in a comment counts")
	}

	// Indexing the file again replaces what it said.
	d.IndexFile("file:///app/app/Http/Controllers/UserController.php", "<?php return view('users.show');")
	if vars, _ := d.Vars("users.show"); len(vars) != 0 || !d.RenderedFrom("users.show") {
		t.Errorf("after reindexing: %v", vars)
	}
	if d.RenderedFrom("users.index") {
		t.Error("users.index is no longer rendered")
	}
}

func TestViewDataThroughTemplates(t *testing.T) {
	d := NewViewData("/app")
	d.IndexFile("file:///app/app/Http/Controllers/PageController.php", `<?php return view('pages.home', ['user' => $u]);`)
	d.IndexFile("file:///app/resources/views/pages/home.blade.php",
		"@extends('layouts.app')\n@include('partials.card', ['title' => $user->name])\n@each('partials.row', $items, 'row')")

	if got := describe(mustVars(d, "layouts.app")); got != "user=$u@0:34" {
		t.Errorf("layout: %q", got)
	}
	if got := describe(mustVars(d, "partials.card")); got != "title=$user->name@1:28 user=$u@0:34" {
		t.Errorf("partial: %q", got)
	}
	if got := describe(mustVars(d, "partials.row")); got != "key=@2:31 row=@2:31" {
		t.Errorf("@each view gets only its item: %q", got)
	}
	if d.ViewName("file:///app/resources/views/pages/home.blade.php") != "pages.home" {
		t.Error("ViewName")
	}
}

func mustVars(d *ViewData, view string) []ViewVar {
	vars, _ := d.Vars(view)
	return vars
}
