package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestBackgroundAnswerLeavesTheLoopFreeAndChecksTheDocument(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	path := filepath.Join(t.TempDir(), "background.php")
	saveFile := func(text string) {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	saveFile("<?php\n")
	uri := symbols.PathToURI(path)
	setDocument := func(text string) {
		h.server.docMu.Lock()
		h.server.documents[uri] = text
		h.server.docMu.Unlock()
	}
	setDocument("<?php\n")
	closeDocument := func() {
		h.server.docMu.Lock()
		delete(h.server.documents, uri)
		h.server.docMu.Unlock()
	}
	for _, c := range []struct {
		id      string
		changed bool
		closed  bool
	}{{"900", false, false}, {"901", true, false}, {"902", false, true}, {"903", true, true}} {
		release := make(chan struct{})
		id := json.RawMessage(c.id)
		h.server.answerInBackground(&jsonRPCMessage{ID: &id, Method: "textDocument/references"}, bladeRequest{uri: uri, source: "<?php\n"}, func() any {
			<-release
			return []string{"answer"}
		})
		if resp := h.readResponse(h.send("tusk/noSuchMethod", nil)); resp["error"] == nil {
			t.Fatalf("the message loop answers while the background request runs, got %v", resp)
		}
		if c.changed {
			setDocument("<?php\n$a = 1;\n")
		}
		if c.closed {
			if c.changed {
				saveFile("<?php\n$a = 1;\n")
			}
			closeDocument()
		}
		close(release)
		var want int
		json.Unmarshal(id, &want)
		resp := h.readResponse(want)
		switch rpcErr, _ := resp["error"].(map[string]interface{}); {
		case c.changed && (rpcErr == nil || rpcErr["code"] != float64(codeContentModified)):
			t.Errorf("an answer for a document that changed (closed: %v) is ContentModified, got %v", c.closed, resp)
		case !c.changed && resp["result"] == nil:
			t.Errorf("an answer for an unchanged document (closed: %v) is sent, got %v", c.closed, resp)
		}
		setDocument("<?php\n")
		saveFile("<?php\n")
	}
}
