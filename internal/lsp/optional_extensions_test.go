package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestedExtensionsReachLSPIndex(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require":{"php":"^7.4"},"suggest":{"ext-imagick":"Images","ext-ssh2":"Remote files"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t)
	defer h.close()
	id := h.send("initialize", map[string]interface{}{
		"rootUri":      "file://" + root,
		"capabilities": map[string]interface{}{},
	})
	if resp := h.readResponse(id); resp["error"] != nil {
		t.Fatalf("initialize: %v", resp)
	}
	for _, name := range []string{"Imagick::readImage", "ssh2_connect"} {
		if h.server.index.Lookup(name) == nil {
			t.Errorf("missing %s from LSP index", name)
		}
	}
	if h.server.index.Lookup("Redis") != nil {
		t.Error("unselected Redis extension was indexed")
	}
}
