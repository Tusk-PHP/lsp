package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestModelMembersSurviveOpeningTheModel opens a model file, then edits it,
// and checks that completion on the model still offers its database columns
// and relations: re-indexing the file must not lose what model analysis
// added to it.
func TestModelMembersSurviveOpeningTheModel(t *testing.T) {
	requireLaravelVendor(t)
	root, err := filepath.Abs("../../testdata/laravel")
	if err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(root, "app/Models/Product.php")
	data, err := os.ReadFile(modelPath)
	if err != nil {
		t.Skip("testdata/laravel isn't available")
	}
	modelSrc := string(data)
	modelURI := "file://" + modelPath

	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": "file://" + root, "capabilities": map[string]interface{}{}}))
	h.notify("initialized", map[string]interface{}{})

	useURI := "file://" + filepath.Join(root, "app/Http/Controllers/ProductProbeController.php")
	useSrc := `<?php
namespace App\Http\Controllers;

use App\Models\Product;

class ProductProbeController
{
    public function show(Product $product)
    {
        $product->
    }
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": useURI, "languageId": "php", "version": 1, "text": useSrc},
	})
	members := func() string {
		return responseText(h.readResponse(h.send("textDocument/completion", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": useURI},
			"position":     map[string]interface{}{"line": 9, "character": 18},
		})))
	}
	columns := []string{"category_id", "created_at", "description", "id", "price", "published", "stock", "title", "updated_at"}
	missing := func(got string, want ...string) []string {
		var out []string
		for _, name := range want {
			if !strings.Contains(got, `"label":"`+name+`"`) {
				out = append(out, name)
			}
		}
		return out
	}

	// Model analysis runs in the background once indexing is done.
	deadline := time.Now().Add(30 * time.Second)
	for len(missing(members(), columns...)) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("model analysis never added Product's columns: missing %v", missing(members(), columns...))
		}
		time.Sleep(20 * time.Millisecond)
	}

	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": modelURI, "languageId": "php", "version": 1, "text": modelSrc},
	})
	if m := missing(members(), append(columns, "category")...); len(m) > 0 {
		t.Errorf("after opening Product.php, completion lost %v", m)
	}

	// An edit that adds a relation: it shows up, and nothing is lost.
	edited := strings.Replace(modelSrc, "    public function category(): BelongsTo", `    public function supplier(): BelongsTo
    {
        return $this->belongsTo(Category::class);
    }

    public function category(): BelongsTo`, 1)
	if edited == modelSrc {
		t.Fatal("Product.php no longer has the category() relation this test edits next to")
	}
	h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument":   map[string]interface{}{"uri": modelURI, "version": 2},
		"contentChanges": []map[string]interface{}{{"text": edited}},
	})
	if m := missing(members(), append(columns, "category", "supplier")...); len(m) > 0 {
		t.Errorf("after editing Product.php, completion lacks %v", m)
	}
}
