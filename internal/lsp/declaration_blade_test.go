package lsp

import (
	"strings"
	"testing"
)

func TestDeclarationInBladeViews(t *testing.T) {
	h, _, uri := viewsWorkspace(t, pageViews(), "page.blade.php")
	defer h.close()
	var locs []testLocation
	request(t, h, "textDocument/declaration", uri, posOf(t, bladeTestPage, "partials.nav", 2), nil, &locs)
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "partials/nav.blade.php") {
		t.Errorf("a view name's declaration is its file, got %+v", locs)
	}

	h2, modelURI, viewURI := bladeWorkspace(t, bladeTestView)
	defer h2.close()
	var loc testLocation
	request(t, h2, "textDocument/declaration", viewURI, posOf(t, bladeTestView, "fullName", 2), nil, &loc)
	if loc.URI != modelURI {
		t.Errorf("a method called in a view is declared in the model, got %+v", loc)
	}
	resp := h2.readResponse(h2.send("textDocument/declaration", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": viewURI},
		"position":     posOf(t, bladeTestView, "profile", 1),
	}))
	if resp["result"] != nil {
		t.Errorf("HTML has no declaration, got %v", resp["result"])
	}
}

func TestDeclarationRejectsInvalidParams(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", initParams()))
	resp := h.readResponse(h.send("textDocument/declaration", "not an object"))
	errObj, _ := resp["error"].(map[string]interface{})
	if errObj == nil || errObj["code"] != float64(-32602) {
		t.Errorf("got %v", resp)
	}
}
