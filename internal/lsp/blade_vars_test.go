package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const varsController = `<?php

namespace App\Http\Controllers;

use App\Models\User;

class ProfileController
{
    public function show(User $user)
    {
        $tab = 'posts';
        return view('profile.show', ['user' => $user])->with(compact('tab'));
    }
}
`

const varsView = `@extends('layouts.app')
@section('content')
    <h1>{{ $user->fullName() }}</h1>
    <p>{{ $tab }}</p>
    {{ $u }}
@endsection
`

const varsLayout = `<title>{{ $user->name }}</title>
@yield('content')
`

// varsWorkspace starts a server on a Laravel app whose controller passes a
// User to a view extending a layout, and opens the view and the layout.
func varsWorkspace(t *testing.T) (h *lspHarness, root, viewURI, layoutURI string) {
	t.Helper()
	root = t.TempDir()
	files := map[string]string{
		"artisan":             "",
		"composer.json":       `{"require": {"laravel/framework": "^11.0"}, "autoload": {"psr-4": {"App\\": "app/"}}}`,
		"app/Models/User.php": bladeTestModel,
		"app/Http/Controllers/ProfileController.php": varsController,
		"resources/views/profile/show.blade.php":     varsView,
		"resources/views/layouts/app.blade.php":      varsLayout,
	}
	for rel, text := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h = newHarness(t)
	h.readResponse(h.send("initialize", map[string]interface{}{
		"rootUri": "file://" + root, "capabilities": map[string]interface{}{}, "processId": nil,
	}))
	h.notify("initialized", map[string]interface{}{})
	time.Sleep(500 * time.Millisecond)
	viewURI = "file://" + filepath.ToSlash(filepath.Join(root, "resources", "views", "profile", "show.blade.php"))
	layoutURI = "file://" + filepath.ToSlash(filepath.Join(root, "resources", "views", "layouts", "app.blade.php"))
	for uri, text := range map[string]string{viewURI: varsView, layoutURI: varsLayout} {
		h.notify("textDocument/didOpen", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri, "languageId": "blade", "version": 1, "text": text},
		})
	}
	time.Sleep(200 * time.Millisecond)
	return h, root, viewURI, layoutURI
}

func TestBladeViewVariableHover(t *testing.T) {
	h, _, viewURI, _ := varsWorkspace(t)
	defer h.close()

	var hover struct{ Contents struct{ Value string } }
	request(t, h, "textDocument/hover", viewURI, posOf(t, varsView, "$user", 2), nil, &hover)
	v := hover.Contents.Value
	if !strings.Contains(v, "User") || !strings.Contains(v, "app/Http/Controllers/ProfileController.php:12") {
		t.Fatalf("expected $user typed User and passed from the controller, got %q", v)
	}

	hover.Contents.Value = ""
	request(t, h, "textDocument/hover", viewURI, posOf(t, varsView, "$tab", 2), nil, &hover)
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Fatalf("expected $tab typed string, got %q", hover.Contents.Value)
	}
}

func TestBladeViewVariableCompletion(t *testing.T) {
	h, _, viewURI, layoutURI := varsWorkspace(t)
	defer h.close()

	var items []struct{ Label, Detail, SortText string }
	request(t, h, "textDocument/completion", viewURI, posOf(t, varsView, "$u }}", 2), nil, &items)
	if len(items) == 0 || items[0].Label != "$user" && !hasFirst(items, "$user") {
		t.Fatalf("expected $user listed first, got %+v", items)
	}

	// Member completion on a passed variable, in the layout the view extends.
	view := strings.Replace(varsLayout, "$user->name", "$user->fu", 1)
	h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument":   map[string]interface{}{"uri": layoutURI, "version": 2},
		"contentChanges": []map[string]interface{}{{"text": view}},
	})
	time.Sleep(100 * time.Millisecond)
	var members []struct{ Label string }
	request(t, h, "textDocument/completion", layoutURI, posOf(t, view, "$user->fu", len("$user->fu")), nil, &members)
	for _, m := range members {
		if m.Label == "fullName" {
			return
		}
	}
	t.Fatalf("expected fullName on the layout's $user, got %+v", members)
}

func hasFirst(items []struct{ Label, Detail, SortText string }, label string) bool {
	best := ""
	bestLabel := ""
	for _, it := range items {
		key := it.SortText
		if key == "" {
			key = it.Label
		}
		if best == "" || key < best {
			best, bestLabel = key, it.Label
		}
	}
	return bestLabel == label
}

func TestBladeViewVariableDefinition(t *testing.T) {
	h, root, viewURI, _ := varsWorkspace(t)
	defer h.close()

	var locs []testLocation
	request(t, h, "textDocument/definition", viewURI, posOf(t, varsView, "$user", 2), nil, &locs)
	controller := "file://" + filepath.ToSlash(filepath.Join(root, "app", "Http", "Controllers", "ProfileController.php"))
	want := posOf(t, varsController, "'user'", 1)
	if len(locs) != 1 || locs[0].URI != controller || locs[0].Range.Start.Line != want["line"] || locs[0].Range.Start.Character != want["character"] {
		t.Fatalf("expected the 'user' key at %v, got %+v", want, locs)
	}
}
