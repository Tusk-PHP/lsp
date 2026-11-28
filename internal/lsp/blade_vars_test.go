package lsp

import (
	"encoding/json"
	"fmt"
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

func TestBladeUndefinedVariableWhenRendersAreKnown(t *testing.T) {
	root := t.TempDir()
	view := `@inject('metrics', 'App\Services\Metrics')
<h1>{{ $user->name }}</h1>
@foreach([$user->name] as $post)
    {{ $loop->index }} {{ $post }}
@endforeach
@error('name') {{ $message }} @enderror
{{ $errors->count() }} {{ $metrics }}
{{ $usr }}
`
	files := map[string]string{
		"artisan":             "",
		"composer.json":       `{"require": {"laravel/framework": "^11.0"}, "autoload": {"psr-4": {"App\\": "app/"}}}`,
		"app/Models/User.php": bladeTestModel,
		"app/Http/Controllers/ProfileController.php": `<?php
namespace App\Http\Controllers;

use App\Models\User;

class ProfileController
{
    public function show(User $user)
    {
        return view('profile.show', ['user' => $user]);
    }
}
`,
		"resources/views/profile/show.blade.php": view,
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
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": "file://" + root, "capabilities": map[string]interface{}{}, "processId": nil}))
	h.notify("initialized", map[string]interface{}{})
	time.Sleep(500 * time.Millisecond)
	uri := "file://" + filepath.ToSlash(filepath.Join(root, "resources", "views", "profile", "show.blade.php"))
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "blade", "version": 1, "text": view},
	})

	deadline := time.Now().Add(8 * time.Second)
	var last []string
	for time.Now().Before(deadline) {
		msg := h.readNotification("textDocument/publishDiagnostics", 8*time.Second)
		raw, _ := json.Marshal(msg["params"])
		var params struct {
			URI         string
			Diagnostics []struct {
				Code, Message string
				Range         testRange
			}
		}
		_ = json.Unmarshal(raw, &params)
		if params.URI != uri {
			continue
		}
		last = last[:0]
		for _, d := range params.Diagnostics {
			last = append(last, fmt.Sprintf("%s: %s @%d:%d", d.Code, d.Message, d.Range.Start.Line, d.Range.Start.Character))
		}
		if strings.Join(last, "\n") == "undefined-variable: Undefined variable $usr @7:3" {
			return
		}
	}
	t.Fatalf("expected only $usr reported, last got %q", last)
}

func TestBladeComponentAndMailableVariables(t *testing.T) {
	root := t.TempDir()
	alertView := "<div class=\"{{ $type }}\">{{ $isActive() }} {{ $title }}</div>\n"
	mailView := "Hello {{ $order->id }}\n"
	files := map[string]string{
		"artisan":       "",
		"composer.json": `{"require": {"laravel/framework": "^11.0"}, "autoload": {"psr-4": {"App\\": "app/"}}}`,
		"vendor/laravel/framework/src/Illuminate/View/Component.php": "<?php\nnamespace Illuminate\\View;\nabstract class Component { public $componentName; public function render() {} }\n",
		"vendor/laravel/framework/src/Illuminate/Mail/Mailable.php":  "<?php\nnamespace Illuminate\\Mail;\nclass Mailable { public $subject; }\n",
		"app/Models/Order.php": "<?php\nnamespace App\\Models;\nclass Order { public int $id = 0; }\n",
		"app/View/Components/Alert.php": `<?php
namespace App\View\Components;

use Illuminate\View\Component;

class Alert extends Component
{
    public function __construct(public string $type = 'info', public string $title = '') {}

    public function isActive(): bool { return true; }

    public function render() { return view('components.alert'); }
}
`,
		"app/Mail/OrderShipped.php": `<?php
namespace App\Mail;

use App\Models\Order;
use Illuminate\Mail\Mailable;

class OrderShipped extends Mailable
{
    public Order $order;

    public function content(): Content
    {
        return new Content(view: 'mail.shipped');
    }
}
`,
		"resources/views/components/alert.blade.php": alertView,
		"resources/views/mail/shipped.blade.php":     mailView,
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
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": "file://" + root, "capabilities": map[string]interface{}{}, "processId": nil}))
	h.notify("initialized", map[string]interface{}{})
	time.Sleep(800 * time.Millisecond)

	hover := func(rel, text, needle string) string {
		uri := "file://" + filepath.ToSlash(filepath.Join(root, filepath.FromSlash(rel)))
		h.notify("textDocument/didOpen", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri, "languageId": "blade", "version": 1, "text": text},
		})
		var out struct{ Contents struct{ Value string } }
		request(t, h, "textDocument/hover", uri, posOf(t, text, needle, 2), nil, &out)
		return out.Contents.Value
	}
	if v := hover("resources/views/components/alert.blade.php", alertView, "$type"); !strings.Contains(v, "string") || !strings.Contains(v, "app/View/Components/Alert.php") {
		t.Errorf("a component's public property: %q", v)
	}
	if v := hover("resources/views/components/alert.blade.php", alertView, "$isActive"); !strings.Contains(v, "Closure") {
		t.Errorf("a component's public method: %q", v)
	}
	if v := hover("resources/views/mail/shipped.blade.php", mailView, "$order"); !strings.Contains(v, "Order $order") || !strings.Contains(v, "app/Mail/OrderShipped.php") {
		t.Errorf("a Mailable's public property: %q", v)
	}
	// The property's class is resolved through the Mailable's imports.
	uri := "file://" + filepath.ToSlash(filepath.Join(root, "resources", "views", "mail", "shipped.blade.php"))
	var member struct{ Contents struct{ Value string } }
	request(t, h, "textDocument/hover", uri, posOf(t, mailView, "->id", 2), nil, &member)
	if !strings.Contains(member.Contents.Value, "int") {
		t.Errorf("$order->id should resolve through App\\Models\\Order: %q", member.Contents.Value)
	}
}

func TestAssignedInFindsWhatATemplateAssignsItself(t *testing.T) {
	for _, c := range []struct {
		src  string
		want bool
	}{
		{"@php $user = 1; @endphp", true},
		{"@foreach ($rows as $user)", true},
		{"@foreach ($rows as $key => $user)", true},
		{"{{ $user }}", false},
		{"@if ($user == 1)", false},
		{"{{ ['name' => $user] }}", false},
		{"@foreach ($rows as $key => $other) {{ $user }}", false},
		{"@php $users = 1; @endphp {{ $user }}", false},
	} {
		if got := assignedIn(c.src, "user"); got != c.want {
			t.Errorf("assignedIn(%q, user) = %v, want %v", c.src, got, c.want)
		}
	}
}
