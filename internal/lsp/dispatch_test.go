package lsp

import (
	"encoding/json"
	"io"
	"log"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

var capabilityRequests = map[string][]string{
	"completionProvider":        {"textDocument/completion", "completionItem/resolve"},
	"hoverProvider":             {"textDocument/hover"},
	"definitionProvider":        {"textDocument/definition"},
	"typeHierarchyProvider":     {"textDocument/prepareTypeHierarchy", "typeHierarchy/supertypes", "typeHierarchy/subtypes"},
	"documentLinkProvider":      {"textDocument/documentLink"},
	"declarationProvider":       {"textDocument/declaration"},
	"typeDefinitionProvider":    {"textDocument/typeDefinition"},
	"implementationProvider":    {"textDocument/implementation"},
	"referencesProvider":        {"textDocument/references"},
	"documentSymbolProvider":    {"textDocument/documentSymbol"},
	"documentHighlightProvider": {"textDocument/documentHighlight"},
	"foldingRangeProvider":      {"textDocument/foldingRange"},
	"workspaceSymbolProvider":   {"workspace/symbol"},
	"signatureHelpProvider":     {"textDocument/signatureHelp"},
	"renameProvider":            {"textDocument/prepareRename", "textDocument/rename"},
	"codeActionProvider":        {"textDocument/codeAction"},
	"executeCommandProvider":    {"workspace/executeCommand"},
	"inlayHintProvider":         {"textDocument/inlayHint"},
	"selectionRangeProvider":    {"textDocument/selectionRange"},
	"semanticTokensProvider":    {"textDocument/semanticTokens/full"},
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

func TestEveryAdvertisedCommandIsHandled(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	caps := h.readResponse(h.send("initialize", initParams()))["result"].(map[string]interface{})["capabilities"].(map[string]interface{})
	h.notify("initialized", map[string]interface{}{})
	advertised := caps["executeCommandProvider"].(map[string]interface{})["commands"].([]interface{})
	if len(advertised) != len(protocol.Commands) {
		t.Fatalf("advertised %v, want every command in protocol.Commands", advertised)
	}
	kinds := caps["codeActionProvider"].(map[string]interface{})["codeActionKinds"].([]interface{})
	if len(kinds) != len(protocol.CodeActionKinds) {
		t.Errorf("advertised kinds %v, want every kind in protocol.CodeActionKinds", kinds)
	}
	for _, command := range advertised {
		resp := h.readResponse(h.send("workspace/executeCommand", map[string]interface{}{"command": command}))
		errObj, _ := resp["error"].(map[string]interface{})
		if message, _ := errObj["message"].(string); message == "Unknown command: "+command.(string) {
			t.Errorf("%s is advertised but not handled", command)
		}
	}
}

func TestDeclarationThroughTheServer(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", initParams()))
	h.notify("initialized", map[string]interface{}{})
	uri := rootURI() + "/declaration_probe.php"
	text := "<?php\ninterface Probe { public function run(): void; }\nclass ProbeImpl implements Probe { public function run(): void {} }\nfunction go_(ProbeImpl $p) { $p->run(); }\n"
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": text},
	})
	resp := h.readResponse(h.send("textDocument/declaration", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 3, "character": 34},
	}))
	loc, _ := resp["result"].(map[string]interface{})
	if loc == nil || loc["range"].(map[string]interface{})["start"].(map[string]interface{})["line"] != float64(1) {
		t.Errorf("want the interface's run on line 1, got %v", resp["result"])
	}
}

func TestMalformedParamsAreRejectedAsInvalidParams(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", initParams()))
	h.notify("initialized", map[string]interface{}{})

	resp := h.readResponse(h.send("textDocument/hover", "not an object"))
	assertInvalidParams(t, resp, "textDocument/hover")
}

func TestMalformedNotificationGetsNoReply(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", initParams()))
	h.notify("initialized", map[string]interface{}{})

	h.notify("textDocument/didOpen", "not an object")
	id := h.send("shutdown", nil)
	timeout := time.After(5 * time.Second)
	for {
		select {
		case msg := <-h.responses:
			respID, ok := msg["id"].(float64)
			if !ok {
				t.Fatalf("malformed didOpen got a reply: %v", msg)
			}
			if int(respID) == id {
				return
			}
		case <-timeout:
			t.Fatalf("timeout waiting for response id=%d", id)
		}
	}
}

func TestCommandArgumentsAreValidated(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", initParams()))
	h.notify("initialized", map[string]interface{}{})

	uri := rootURI() + "/missing.php"
	cases := []struct {
		command   protocol.CommandName
		arguments []interface{}
		want      string
	}{
		{protocol.CommandCopyNamespace, nil, "tuskPhpLsp.copyNamespace takes argument 1, got 0 arguments"},
		{protocol.CommandMoveToNamespace, []interface{}{uri}, "tuskPhpLsp.moveToNamespace takes argument 2, got 1 arguments"},
		{protocol.CommandCreateObject, []interface{}{uri, "class", 3}, "tuskPhpLsp.createObject argument 3"},
		{protocol.CommandCopyReference, []interface{}{uri, "line 3"}, "tuskPhpLsp.copyReference argument 2"},
	}
	for _, tc := range cases {
		t.Run(string(tc.command), func(t *testing.T) {
			resp := h.readResponse(h.send("workspace/executeCommand", map[string]interface{}{
				"command":   tc.command,
				"arguments": tc.arguments,
			}))
			assertInvalidParams(t, resp, tc.want)
		})
	}
}

func assertInvalidParams(t *testing.T, resp map[string]interface{}, want string) {
	t.Helper()
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected an error response, got %v", resp)
	}
	if code, _ := errObj["code"].(float64); code != codeInvalidParams {
		t.Errorf("error code = %v, want %d", errObj["code"], codeInvalidParams)
	}
	if message, _ := errObj["message"].(string); !strings.Contains(message, want) {
		t.Errorf("error message = %q, want it to contain %q", message, want)
	}
}

func BenchmarkDecodeParams(b *testing.B) {
	server := NewServer(strings.NewReader(""), io.Discard, log.New(io.Discard, "", 0))
	msg := &jsonRPCMessage{
		Method: "textDocument/completion",
		Params: json.RawMessage(`{"textDocument":{"uri":"file:///app/Models/User.php"},"position":{"line":12,"character":8},"context":{"triggerKind":2,"triggerCharacter":">"}}`),
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := server.decodeParams[completionParams](msg); !ok {
			b.Fatal("decode failed")
		}
	}
}
