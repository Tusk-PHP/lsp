package lsp

import "testing"

func TestBladeViewOffersIgnoreQuickFix(t *testing.T) {
	h, _, viewURI := bladeWorkspace(t, "<div>\n    {{ missing_helper() }}\n</div>\n")
	defer h.close()

	diag := map[string]interface{}{
		"range":   map[string]interface{}{"start": map[string]interface{}{"line": 1, "character": 7}, "end": map[string]interface{}{"line": 1, "character": 21}},
		"source":  "tusk-php",
		"code":    "unknown-function",
		"message": "Unknown function missing_helper",
	}
	resp := h.readResponse(h.send("textDocument/codeAction", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": viewURI},
		"range":        diag["range"],
		"context":      map[string]interface{}{"diagnostics": []interface{}{diag}},
	}))
	actions, _ := resp["result"].([]interface{})
	if len(actions) != 1 {
		t.Fatalf("want the ignore quick fix, got %v", resp)
	}
	edit := actions[0].(map[string]interface{})["edit"].(map[string]interface{})["changes"].(map[string]interface{})[viewURI].([]interface{})[0].(map[string]interface{})
	if edit["newText"] != "    {{-- @tusk-ignore unknown-function --}}\n" {
		t.Errorf("got %v", edit)
	}
}
