package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEloquentStaticCallsRightAfterStartup asks about an Eloquent model the
// moment the server has indexed the project, as an editor that opens a file
// does, before the background model analysis has run: Category::where() and
// Category::find() are forwarded to the query builder, which the index
// already knows.
func TestEloquentStaticCallsRightAfterStartup(t *testing.T) {
	requireLaravelVendor(t)
	root, err := filepath.Abs("../../testdata/laravel")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "app/Http/Controllers/CategoryController.php")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("testdata/laravel isn't available")
	}
	src := string(data)
	uri := "file://" + path

	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": "file://" + root, "capabilities": map[string]interface{}{}}))
	h.notify("initialized", map[string]interface{}{})
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": src},
	})
	at := func(anchor string, offset int) map[string]interface{} {
		i := strings.Index(src, anchor)
		if i < 0 {
			t.Fatalf("anchor %q not found", anchor)
		}
		i += offset
		line := strings.Count(src[:i], "\n")
		col := i - strings.LastIndex(src[:i], "\n") - 1
		return map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri},
			"position":     map[string]interface{}{"line": line, "character": col},
		}
	}

	// Wait until the project and its vendor folder are indexed, which is when
	// the server reports itself ready, then ask straight away. The model
	// analysis that injects relation properties starts only then, in the
	// background, so it hasn't run yet.
	deadline := time.Now().Add(20 * time.Second)
	for !h.server.index.Ready() {
		if time.Now().After(deadline) {
			t.Fatal("the index never became ready")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The same positions as the benchmark's Eloquent cases.
	if got := responseText(h.readResponse(h.send("textDocument/completion", at("Category::first();", 10)))); !strings.Contains(got, `"label":"where"`) || !strings.Contains(got, `"label":"orderBy"`) {
		t.Errorf("Category:: completion is missing the builder's where() or the query builder's orderBy(): %.600s", got)
	}
	if got := responseText(h.readResponse(h.send("textDocument/hover", at("Category::find(1)", 12)))); !strings.Contains(got, "find") {
		t.Errorf("hover on Category::find(): %s", got)
	}
	if got := responseText(h.readResponse(h.send("textDocument/hover", at("->orderBy('name')", 4)))); !strings.Contains(got, "orderBy") {
		t.Errorf("hover on ->orderBy() through the builder's @mixin: %s", got)
	}
	if got := responseText(h.readResponse(h.send("textDocument/completion", at("$category->products;", 11)))); !strings.Contains(got, `"label":"products"`) {
		t.Errorf("$category-> completion is missing products")
	}
	if got := responseText(h.readResponse(h.send("textDocument/definition", at("$category->products;", 13)))); !strings.Contains(got, "Category.php") {
		t.Errorf("definition of $category->products: %s", got)
	}
}

// TestEloquentLocalScopesThroughTheServer checks hover and go to definition
// on local scopes, called statically on the model and on its query builder.
// The model and the code using it are open, unsaved documents.
func TestEloquentLocalScopesThroughTheServer(t *testing.T) {
	root, err := filepath.Abs("../../testdata/laravel")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "vendor")); err != nil {
		t.Skip("testdata/laravel isn't available")
	}
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": "file://" + root, "capabilities": map[string]interface{}{}}))
	h.notify("initialized", map[string]interface{}{})
	deadline := time.Now().Add(20 * time.Second)
	for !h.server.index.Ready() {
		if time.Now().After(deadline) {
			t.Fatal("the index never became ready")
		}
		time.Sleep(10 * time.Millisecond)
	}

	modelURI := "file://" + filepath.Join(root, "app/Models/ScopedPost.php")
	model := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Attributes\Scope;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Database\Eloquent\Model;

class ScopedPost extends Model
{
    /** Only posts that are out. */
    public function scopePublished(Builder $query): void {}

    /** Posts with many votes. */
    #[Scope]
    protected function popular(Builder $query): void {}
}
`
	useURI := "file://" + filepath.Join(root, "app/Http/Controllers/ScopedPostController.php")
	use := `<?php
namespace App\Http\Controllers;

use App\Models\ScopedPost;

class ScopedPostController
{
    public function index()
    {
        $a = ScopedPost::published()->get();
        $b = ScopedPost::query()->popular()->first();
    }
}
`
	for uri, text := range map[string]string{modelURI: model, useURI: use} {
		h.notify("textDocument/didOpen", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": text},
		})
	}
	at := func(anchor string, offset int) map[string]interface{} {
		i := strings.Index(use, anchor) + offset
		return map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": useURI},
			"position":     map[string]interface{}{"line": strings.Count(use[:i], "\n"), "character": i - strings.LastIndex(use[:i], "\n") - 1},
		}
	}
	if got := responseText(h.readResponse(h.send("textDocument/hover", at("published()", 2)))); !strings.Contains(got, "Only posts that are out") {
		t.Errorf("hover on ScopedPost::published(): %s", got)
	}
	if got := responseText(h.readResponse(h.send("textDocument/hover", at("popular()", 2)))); !strings.Contains(got, "Posts with many votes") {
		t.Errorf("hover on ->popular(): %s", got)
	}
	for _, anchor := range []string{"published()", "popular()"} {
		if got := responseText(h.readResponse(h.send("textDocument/definition", at(anchor, 2)))); !strings.Contains(got, "ScopedPost.php") {
			t.Errorf("definition of %s: %s", anchor, got)
		}
	}
}
