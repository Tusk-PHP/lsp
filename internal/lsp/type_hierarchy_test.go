package lsp

import (
	"testing"
)

func TestTypeHierarchyThroughTheServer(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", initParams()))
	h.notify("initialized", map[string]interface{}{})
	uri := rootURI() + "/hierarchy_probe.php"
	text := "<?php\ninterface HProbe {}\nclass HBase implements HProbe {}\nclass HLeaf extends HBase {}\n"
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": text},
	})
	resp := h.readResponse(h.send("textDocument/prepareTypeHierarchy", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 3, "character": 22},
	}))
	items, _ := resp["result"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("prepare: %v", resp)
	}
	base := items[0].(map[string]interface{})
	if base["name"] != "HBase" || base["data"].(map[string]interface{})["fqn"] != "HBase" {
		t.Fatalf("item: %v", base)
	}
	for method, want := range map[string]string{"typeHierarchy/supertypes": "HProbe", "typeHierarchy/subtypes": "HLeaf"} {
		resp := h.readResponse(h.send(method, map[string]interface{}{"item": base}))
		got, _ := resp["result"].([]interface{})
		if len(got) != 1 || got[0].(map[string]interface{})["name"] != want {
			t.Errorf("%s: got %v, want %s", method, resp["result"], want)
		}
	}
	for _, method := range []string{"textDocument/prepareTypeHierarchy", "typeHierarchy/supertypes"} {
		resp := h.readResponse(h.send(method, "not an object"))
		if errObj, _ := resp["error"].(map[string]interface{}); errObj == nil || errObj["code"] != float64(-32602) {
			t.Errorf("%s with invalid params: %v", method, resp)
		}
	}
}

func TestTypeHierarchyInBladeHTML(t *testing.T) {
	h, _, viewURI := bladeWorkspace(t, bladeTestView)
	defer h.close()
	resp := h.readResponse(h.send("textDocument/prepareTypeHierarchy", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": viewURI},
		"position":     posOf(t, bladeTestView, "profile", 1),
	}))
	if resp["result"] != nil {
		t.Errorf("HTML has no type hierarchy, got %v", resp["result"])
	}
}
