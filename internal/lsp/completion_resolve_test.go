package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A client that resolves documentation later gets member items without it,
// and completionItem/resolve fills it in. Other clients get it up front.
func TestCompletionResolveFillsDocumentation(t *testing.T) {
	src := "<?php\nclass Svc {\n    /** Sends the mail. */\n    public function send(): void {}\n}\nfunction run(Svc $s): void {\n    $s->\n}\n"
	for _, lazy := range []bool{true, false} {
		root := t.TempDir()
		path := filepath.Join(root, "a.php")
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		uri := "file://" + path
		caps := map[string]interface{}{}
		if lazy {
			caps = map[string]interface{}{"textDocument": map[string]interface{}{"completion": map[string]interface{}{
				"completionItem": map[string]interface{}{"resolveSupport": map[string]interface{}{"properties": []string{"documentation", "detail"}}},
			}}}
		}
		h := newHarness(t)
		h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": "file://" + root, "capabilities": caps}))
		h.notify("initialized", map[string]interface{}{})
		h.notify("textDocument/didOpen", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": src},
		})
		resp := h.readResponse(h.send("textDocument/completion", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri},
			"position":     map[string]interface{}{"line": 6, "character": 8},
		}))
		var item map[string]interface{}
		items, _ := resp["result"].([]interface{})
		for _, raw := range items {
			if it, ok := raw.(map[string]interface{}); ok && it["label"] == "send" {
				item = it
			}
		}
		if item == nil {
			h.close()
			t.Fatalf("lazy=%v: no send item in %v", lazy, responseText(resp))
		}
		doc, _ := item["documentation"].(string)
		if lazy == strings.Contains(doc, "Sends the mail") {
			h.close()
			t.Fatalf("lazy=%v: documentation in list = %q", lazy, doc)
		}
		if lazy {
			resolved := h.readResponse(h.send("completionItem/resolve", item))
			if got := responseText(resolved); !strings.Contains(got, "Sends the mail") || !strings.Contains(got, "detail") {
				h.close()
				t.Fatalf("resolve didn't fill in the item: %s", got)
			}
		}
		h.close()
	}
}
