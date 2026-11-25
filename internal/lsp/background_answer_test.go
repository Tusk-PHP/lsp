package lsp

import (
	"encoding/json"
	"testing"
)

func TestBackgroundAnswerLeavesTheLoopFreeAndChecksTheDocument(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	uri := "file:///tmp/background.php"
	setDocument := func(text string) {
		h.server.docMu.Lock()
		h.server.documents[uri] = text
		h.server.docMu.Unlock()
	}
	setDocument("<?php\n")
	for _, c := range []struct {
		id      string
		changed bool
	}{{"900", false}, {"901", true}} {
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
		close(release)
		var want int
		json.Unmarshal(id, &want)
		resp := h.readResponse(want)
		switch rpcErr, _ := resp["error"].(map[string]interface{}); {
		case c.changed && (rpcErr == nil || rpcErr["code"] != float64(codeContentModified)):
			t.Errorf("an answer for a document that changed is ContentModified, got %v", resp)
		case !c.changed && resp["result"] == nil:
			t.Errorf("an answer for an unchanged document is sent, got %v", resp)
		}
		setDocument("<?php\n")
	}
}
