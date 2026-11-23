package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestUnknownRelationOnSave saves a file that eager-loads relations of a
// model on the Laravel fixture and checks the server reports the relation
// the model doesn't have, and nothing about columns: the fixture's columns
// come from migrations, which may not list every column.
func TestUnknownRelationOnSave(t *testing.T) {
	requireLaravelVendor(t)
	root, err := filepath.Abs("../../testdata/laravel")
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": "file://" + root, "capabilities": map[string]interface{}{}}))
	h.notify("initialized", map[string]interface{}{})

	// Model analysis runs in the background once indexing is done.
	deadline := time.Now().Add(30 * time.Second)
	for h.server.workspace == nil || h.server.index.Lookup(`App\Models\Product::$title`) == nil {
		if time.Now().After(deadline) {
			t.Fatal("model analysis never ran")
		}
		time.Sleep(20 * time.Millisecond)
	}

	uri := "file://" + filepath.Join(root, "app/Http/Controllers/RelationProbeController.php")
	src := `<?php
namespace App\Http\Controllers;

use App\Models\Product;

class RelationProbeController
{
    public function index()
    {
        // Product::with('inAComment')
        Product::with('category')->get();
        Product::with(['category', 'missingRelation'])->get();
        Product::where('missing_column', 1)->get();
    }
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": src},
	})
	h.notify("textDocument/didSave", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})

	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		msg := h.readNotification("textDocument/publishDiagnostics", 10*time.Second)
		raw, _ := json.Marshal(msg["params"])
		var params struct {
			URI         string
			Diagnostics []struct{ Code, Message string }
		}
		_ = json.Unmarshal(raw, &params)
		if params.URI != uri {
			continue
		}
		var got []string
		for _, d := range params.Diagnostics {
			if strings.HasPrefix(d.Code, "unknown-") {
				got = append(got, d.Code+": "+d.Message)
			}
		}
		if len(got) == 0 {
			continue // the publish before the save
		}
		want := "unknown-relation: Unknown relation 'missingRelation' on model 'Product'"
		if strings.Join(got, "\n") != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		return
	}
	t.Fatal("no unknown-relation diagnostic after saving")
}
