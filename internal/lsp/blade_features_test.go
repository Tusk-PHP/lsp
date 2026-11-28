package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const bladeTestLayout = `<html>
<title>@yield('title')</title>
@stack('scripts')
@yield('content')
</html>
`

const bladeTestPage = `@extends('layouts.app')
@section('title', 'Home')
@section('content')
    @include('partials.nav')
    @include('partials.missing')
@endsection
@push('scripts')
@endpush
`

// viewsWorkspace starts a server on a workspace with the views given, by path
// under resources/views, and opens the one named open.
func viewsWorkspace(t *testing.T, views map[string]string, open string) (h *lspHarness, root, uri string) {
	t.Helper()
	root = t.TempDir()
	for name, text := range views {
		path := filepath.Join(root, "resources", "views", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h = newHarness(t)
	h.readResponse(h.send("initialize", map[string]interface{}{
		"rootUri": symbols.PathToURI(root), "capabilities": map[string]interface{}{}, "processId": nil,
	}))
	h.notify("initialized", map[string]interface{}{})
	uri = symbols.PathToURI(filepath.Join(root, "resources", "views", open))
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "blade", "version": 1, "text": views[open]},
	})
	time.Sleep(200 * time.Millisecond)
	return h, root, uri
}

func pageViews() map[string]string {
	return map[string]string{
		"layouts/app.blade.php":  bladeTestLayout,
		"partials/nav.blade.php": "<nav></nav>\n",
		"page.blade.php":         bladeTestPage,
	}
}

type testCompletionItem struct {
	Label            string
	InsertTextFormat int
	TextEdit         struct {
		Range   testRange
		NewText string
	}
}

func TestBladeDirectiveCompletion(t *testing.T) {
	views := pageViews()
	views["page.blade.php"] = "<div>\n    @forea\n</div>\n"
	h, _, uri := viewsWorkspace(t, views, "page.blade.php")
	defer h.close()

	var items []testCompletionItem
	request(t, h, "textDocument/completion", uri, map[string]interface{}{"line": 1, "character": 10}, nil, &items)
	if len(items) != 1 || items[0].Label != "@foreach" || items[0].InsertTextFormat != 2 {
		t.Fatalf("expected the @foreach snippet, got %+v", items)
	}
	edit := items[0].TextEdit
	if edit.Range.Start.Character != 4 || edit.Range.End.Character != 10 || !strings.HasSuffix(edit.NewText, "@endforeach") {
		t.Fatalf("expected the snippet to replace @forea and close the loop, got %+v", edit)
	}
}

func TestBladeViewAndSectionCompletion(t *testing.T) {
	views := pageViews()
	views["page.blade.php"] = "@extends('layouts.app')\n@section('')\n@include('partials.')\n"
	h, _, uri := viewsWorkspace(t, views, "page.blade.php")
	defer h.close()

	var items []testCompletionItem
	request(t, h, "textDocument/completion", uri, map[string]interface{}{"line": 1, "character": len("@section('")}, nil, &items)
	var labels []string
	for _, it := range items {
		labels = append(labels, it.Label)
	}
	if strings.Join(labels, " ") != "title content" {
		t.Fatalf("expected the layout's sections, got %v", labels)
	}

	items = nil
	request(t, h, "textDocument/completion", uri, map[string]interface{}{"line": 2, "character": len("@include('partials.")}, nil, &items)
	if len(items) != 1 || items[0].Label != "partials.nav" {
		t.Fatalf("expected partials.nav, got %+v", items)
	}
}

func TestBladeViewAndSectionDefinition(t *testing.T) {
	h, root, uri := viewsWorkspace(t, pageViews(), "page.blade.php")
	defer h.close()

	layoutURI := symbols.PathToURI(filepath.Join(root, "resources", "views", "layouts", "app.blade.php"))
	var locs []testLocation
	request(t, h, "textDocument/definition", uri, posOf(t, bladeTestPage, "layouts.app", 1), nil, &locs)
	if len(locs) != 1 || locs[0].URI != layoutURI {
		t.Fatalf("expected the layout's file, got %+v", locs)
	}

	locs = nil
	request(t, h, "textDocument/definition", uri, posOf(t, bladeTestPage, "content", 1), nil, &locs)
	want := posOf(t, bladeTestLayout, "content", 0)
	if len(locs) != 1 || locs[0].URI != layoutURI || locs[0].Range.Start.Line != want["line"] || locs[0].Range.Start.Character != want["character"] {
		t.Fatalf("expected @yield('content') in the layout at %v, got %+v", want, locs)
	}

	locs = nil
	request(t, h, "textDocument/definition", uri, posOf(t, bladeTestPage, "scripts", 1), nil, &locs)
	if len(locs) != 1 || locs[0].Range.Start.Line != 2 {
		t.Fatalf("expected @stack('scripts') in the layout, got %+v", locs)
	}
}

func TestBladeDirectiveHover(t *testing.T) {
	h, _, uri := viewsWorkspace(t, pageViews(), "page.blade.php")
	defer h.close()

	var hover struct {
		Contents struct{ Value string }
	}
	request(t, h, "textDocument/hover", uri, posOf(t, bladeTestPage, "@extends", 2), nil, &hover)
	if !strings.Contains(hover.Contents.Value, "layout") {
		t.Fatalf("expected @extends documentation, got %q", hover.Contents.Value)
	}
}

func TestBladeStructureAndUnknownViewDiagnostics(t *testing.T) {
	views := pageViews()
	views["page.blade.php"] = bladeTestPage + "@if($a)\n"
	h, _, uri := viewsWorkspace(t, views, "page.blade.php")
	defer h.close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		msg := h.readNotification("textDocument/publishDiagnostics", 5*time.Second)
		raw, _ := json.Marshal(msg["params"])
		var params struct {
			URI         string
			Diagnostics []struct {
				Code    string
				Message string
				Range   testRange
			}
		}
		_ = json.Unmarshal(raw, &params)
		if params.URI != uri {
			continue
		}
		var got []string
		for _, d := range params.Diagnostics {
			got = append(got, string(d.Code)+": "+d.Message)
		}
		want := "unclosed-directive: `@if` is never closed with `@endif`\nunknown-view: View `partials.missing` not found"
		if strings.Join(got, "\n") == want {
			return
		}
		if len(got) > 0 {
			t.Fatalf("got %q", got)
		}
	}
	t.Fatal("no diagnostics for the view")
}
