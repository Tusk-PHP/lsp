package lsp

import (
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestSavedTextWithLoneCRsIsIndexedOnItsLines(t *testing.T) {
	root := t.TempDir()
	h := newHarness(t)
	t.Cleanup(h.close)
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": symbols.PathToURI(root), "capabilities": map[string]interface{}{}, "processId": nil}))
	h.notify("initialized", map[string]interface{}{})
	waitFor(t, "the index to be ready", h.server.index.Ready)
	uri := symbols.PathToURI(filepath.Join(root, "app", "Saved.php"))
	h.notify("textDocument/didSave", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"text":         "<?php\rnamespace App;\rclass Saved {}\r",
	})
	waitFor(t, "the saved class to be indexed", func() bool { return h.server.index.Lookup(`App\Saved`) != nil })
	if sym := h.server.index.Lookup(`App\Saved`); sym.URI != uri || sym.Range.Start.Line != 2 {
		t.Errorf("the class saved with lone CR line endings is declared on its third line: %s %+v", sym.URI, sym.Range)
	}
}
