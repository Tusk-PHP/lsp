package lsp

import (
	"testing"
	"time"
)

// TestExecuteCommandAdvertisement verifies that all three executeCommand
// commands are listed in the server capabilities returned by initialize,
// and that executing each command does not return an RPC error.
func TestExecuteCommandAdvertisement(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	root := testdataPath()
	initID := h.send("initialize", map[string]interface{}{
		"rootUri":      "file://" + root,
		"capabilities": map[string]interface{}{},
		"processId":    nil,
	})
	resp := h.readResponse(initID)

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("initialize: expected result map, got %T", resp["result"])
	}
	caps, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("initialize: expected capabilities map")
	}

	execProvider, ok := caps["executeCommandProvider"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected executeCommandProvider in capabilities, got %T", caps["executeCommandProvider"])
	}

	rawCmds, ok := execProvider["commands"].([]interface{})
	if !ok {
		t.Fatalf("expected commands array in executeCommandProvider, got %T", execProvider["commands"])
	}

	wantCmds := map[string]bool{
		"tuskPhpLsp.namespaceForPath": false,
		"tuskPhpLsp.copyNamespace":    false,
		"tuskPhpLsp.moveToNamespace":  false,
	}
	for _, raw := range rawCmds {
		cmd, _ := raw.(string)
		if _, ok := wantCmds[cmd]; ok {
			wantCmds[cmd] = true
		}
	}
	for cmd, found := range wantCmds {
		if !found {
			t.Errorf("expected command %q in executeCommandProvider.commands, got %v", cmd, rawCmds)
		}
	}

	// Now initialize the server and test each command doesn't return an RPC error.
	h.notify("initialized", map[string]interface{}{})
	time.Sleep(200 * time.Millisecond)

	uri := "file:///tmp/test_exec_cmd.php"
	source := `<?php
namespace App\Models;
class User {}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": "php",
			"version":    1,
			"text":       source,
		},
	})
	time.Sleep(100 * time.Millisecond)

	// Test tuskPhpLsp.copyNamespace
	copyID := h.send("workspace/executeCommand", map[string]interface{}{
		"command":   "tuskPhpLsp.copyNamespace",
		"arguments": []interface{}{uri},
	})
	copyResp := h.readResponse(copyID)
	if copyResp["error"] != nil {
		t.Errorf("tuskPhpLsp.copyNamespace returned error: %v", copyResp["error"])
	}

	// Test tuskPhpLsp.namespaceForPath
	nsID := h.send("workspace/executeCommand", map[string]interface{}{
		"command":   "tuskPhpLsp.namespaceForPath",
		"arguments": []interface{}{uri},
	})
	nsResp := h.readResponse(nsID)
	if nsResp["error"] != nil {
		t.Errorf("tuskPhpLsp.namespaceForPath returned error: %v", nsResp["error"])
	}

	// Test tuskPhpLsp.moveToNamespace
	moveID := h.send("workspace/executeCommand", map[string]interface{}{
		"command":   "tuskPhpLsp.moveToNamespace",
		"arguments": []interface{}{uri, "App\\Services"},
	})
	moveResp := h.readResponse(moveID)
	if moveResp["error"] != nil {
		t.Errorf("tuskPhpLsp.moveToNamespace returned error: %v", moveResp["error"])
	}
}
