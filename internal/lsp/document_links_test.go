package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestDocumentLinksThroughTheServer(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"bootstrap/app.php", "public/index.php"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("<?php\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": symbols.PathToURI(root), "capabilities": map[string]interface{}{}, "processId": nil}))
	h.notify("initialized", map[string]interface{}{})
	uri := symbols.PathToURI(filepath.Join(root, "public", "index.php"))
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": "<?php\nrequire __DIR__ . '/../bootstrap/app.php';\nrequire __DIR__ . '/../missing.php';\n"},
	})
	resp := h.readResponse(h.send("textDocument/documentLink", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}}))
	links, _ := resp["result"].([]interface{})
	if len(links) != 1 || links[0].(map[string]interface{})["target"] != symbols.PathToURI(filepath.Join(root, "bootstrap", "app.php")) {
		t.Errorf("only the file that exists is linked, got %v", resp["result"])
	}

	blade := symbols.PathToURI(filepath.Join(root, "resources", "views", "a.blade.php"))
	resp = h.readResponse(h.send("textDocument/documentLink", map[string]interface{}{"textDocument": map[string]interface{}{"uri": blade}}))
	if links, ok := resp["result"].([]interface{}); !ok || len(links) != 0 {
		t.Errorf("Blade views have no links yet: %v", resp["result"])
	}
	resp = h.readResponse(h.send("textDocument/documentLink", "not an object"))
	if errObj, _ := resp["error"].(map[string]interface{}); errObj == nil || errObj["code"] != float64(-32602) {
		t.Errorf("invalid params: %v", resp)
	}
}
