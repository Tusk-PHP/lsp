package lsp

import (
	"sort"
	"testing"
)

var capabilityRequests = map[string][]string{
	"completionProvider":         {"textDocument/completion", "completionItem/resolve"},
	"hoverProvider":              {"textDocument/hover"},
	"definitionProvider":         {"textDocument/definition"},
	"typeHierarchyProvider":      {"textDocument/prepareTypeHierarchy", "typeHierarchy/supertypes", "typeHierarchy/subtypes"},
	"typeDefinitionProvider":     {"textDocument/typeDefinition"},
	"implementationProvider":     {"textDocument/implementation"},
	"referencesProvider":         {"textDocument/references"},
	"documentSymbolProvider":     {"textDocument/documentSymbol"},
	"documentHighlightProvider":  {"textDocument/documentHighlight"},
	"foldingRangeProvider":       {"textDocument/foldingRange"},
	"workspaceSymbolProvider":    {"workspace/symbol"},
	"signatureHelpProvider":      {"textDocument/signatureHelp"},
	"renameProvider":             {"textDocument/prepareRename", "textDocument/rename"},
	"codeActionProvider":         {"textDocument/codeAction"},
	"executeCommandProvider":     {"workspace/executeCommand"},
	"inlayHintProvider":          {"textDocument/inlayHint"},
	"selectionRangeProvider":     {"textDocument/selectionRange"},
	"semanticTokensProvider":     {"textDocument/semanticTokens/full"},
	"diagnosticProvider":         {"textDocument/diagnostic"},
	"documentFormattingProvider": {"textDocument/formatting"},
}

var capabilitiesWithoutRequests = map[string]bool{
	"textDocumentSync": true,
	"experimental":     true,
}

func TestEveryAdvertisedCapabilityHasAHandler(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	caps, ok := h.readResponse(h.send("initialize", initParams()))["result"].(map[string]interface{})["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("initialize returned no capabilities")
	}
	h.notify("initialized", map[string]interface{}{})

	names := make([]string, 0, len(caps))
	for name := range caps {
		names = append(names, name)
	}
	sort.Strings(names)

	uri := rootURI() + "/missing.php"
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 0, "character": 0},
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 0, "character": 0},
			"end":   map[string]interface{}{"line": 0, "character": 0},
		},
		"context": map[string]interface{}{"diagnostics": []interface{}{}, "includeDeclaration": true},
		"query":   "",
		"newName": "renamed",
		"command": "",
		"label":   "",
	}

	for _, name := range names {
		if capabilitiesWithoutRequests[name] || caps[name] == false || caps[name] == nil {
			continue
		}
		methods, known := capabilityRequests[name]
		if !known {
			t.Errorf("capability %q is advertised but capabilityRequests does not say which requests serve it", name)
			continue
		}
		for _, method := range methods {
			resp := h.readResponse(h.send(method, params))
			errObj, _ := resp["error"].(map[string]interface{})
			if message, _ := errObj["message"].(string); message == "Method not found: "+method {
				t.Errorf("capability %q is advertised but %s has no handler", name, method)
			}
		}
	}
}
