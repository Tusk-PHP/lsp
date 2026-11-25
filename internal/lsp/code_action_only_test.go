package lsp

import (
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestHandleCodeActionAnswersOnlyTheRequestedKinds(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/code_action_only.php"
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1,
			"text": "<?php\nnamespace App;\n\nuse App\\Unused\\Thing;\n\nclass Order\n{\n    private int $total;\n}\n",
		},
	})
	time.Sleep(200 * time.Millisecond)

	for only, want := range map[string]string{"source": "Copy Namespace: App\\Order", "refactor": "Generate Constructor"} {
		resp := h.readResponse(h.send("textDocument/codeAction", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri},
			"range": map[string]interface{}{
				"start": map[string]interface{}{"line": 5, "character": 0},
				"end":   map[string]interface{}{"line": 5, "character": 0},
			},
			"context": map[string]interface{}{"diagnostics": []interface{}{}, "only": []string{only}},
		}))
		result, ok := resp["result"].([]interface{})
		if !ok {
			t.Fatalf("only %s: expected an array, got %#v", only, resp)
		}
		found := false
		for _, raw := range result {
			action := raw.(map[string]interface{})
			kind, _ := action["kind"].(string)
			if !protocol.CodeActionKind(kind).In([]protocol.CodeActionKind{protocol.CodeActionKind(only)}) {
				t.Errorf("only %s: got an action of kind %s: %v", only, kind, action["title"])
			}
			found = found || action["title"] == want
		}
		if !found {
			t.Errorf("only %s: %q is offered, got %v", only, want, result)
		}
	}
}
