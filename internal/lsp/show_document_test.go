package lsp

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

// initHarnessWithCaps creates an initialized harness using the given capabilities map.
func initHarnessWithCaps(t *testing.T, caps map[string]interface{}) *lspHarness {
	t.Helper()
	h := newHarness(t)
	root := testdataPath()
	initID := h.send("initialize", map[string]interface{}{
		"rootUri":      "file://" + root,
		"capabilities": caps,
		"processId":    nil,
	})
	h.readResponse(initID)
	h.notify("initialized", map[string]interface{}{})
	return h
}

// TestShowDocumentCapabilityTrue asserts that when the client sends
// window.showDocument.support = true, the server records it.
func TestShowDocumentCapabilityTrue(t *testing.T) {
	h := initHarnessWithCaps(t, map[string]interface{}{
		"window": map[string]interface{}{
			"showDocument": map[string]interface{}{
				"support": true,
			},
		},
	})
	defer h.close()

	if !h.server.clientSupportsShowDocument {
		t.Error("expected clientSupportsShowDocument to be true when client advertises support")
	}
}

// TestShowDocumentCapabilityAbsent asserts that when the client omits the
// window capability entirely, clientSupportsShowDocument is false.
func TestShowDocumentCapabilityAbsent(t *testing.T) {
	h := initHarnessWithCaps(t, map[string]interface{}{})
	defer h.close()

	if h.server.clientSupportsShowDocument {
		t.Error("expected clientSupportsShowDocument to be false when window capability is absent")
	}
}

// TestShowDocumentCapabilityExplicitFalse asserts that when the client
// explicitly sets window.showDocument.support = false, the server records false.
func TestShowDocumentCapabilityExplicitFalse(t *testing.T) {
	h := initHarnessWithCaps(t, map[string]interface{}{
		"window": map[string]interface{}{
			"showDocument": map[string]interface{}{
				"support": false,
			},
		},
	})
	defer h.close()

	if h.server.clientSupportsShowDocument {
		t.Error("expected clientSupportsShowDocument to be false when client advertises support=false")
	}
}

// TestSendServerRequestEmitsCorrectFraming calls sendServerRequest and asserts
// the emitted message has correct JSON-RPC framing, method, id, and params.
func TestSendServerRequestEmitsCorrectFraming(t *testing.T) {
	h := initHarnessWithCaps(t, map[string]interface{}{
		"window": map[string]interface{}{
			"showDocument": map[string]interface{}{
				"support": true,
			},
		},
	})
	defer h.close()

	// Drain any notifications already buffered (e.g. publishDiagnostics).
	// We rely on the responses channel for server-to-client requests.
	const wantURI = "https://www.php.net/manual/en/function.array-map.php"

	// Call sendServerRequest directly — it writes to the same output pipe
	// that the harness background reader consumes. Messages with an "id"
	// field land in h.responses.
	h.server.sendServerRequest("window/showDocument", protocol.ShowDocumentParams{
		URI:       wantURI,
		External:  true,
		TakeFocus: true,
	})

	// Read back the server-initiated request from the responses channel.
	timeout := time.After(5 * time.Second)
	var found map[string]interface{}
	for found == nil {
		select {
		case msg := <-h.responses:
			if method, _ := msg["method"].(string); method == "window/showDocument" {
				found = msg
			}
			// put unrelated responses back is not safe with channels; just keep draining
		case <-timeout:
			t.Fatal("timed out waiting for window/showDocument server request")
		}
	}

	// Verify id is a number.
	rawID, hasID := found["id"]
	if !hasID {
		t.Fatal("expected 'id' field in window/showDocument request")
	}
	switch rawID.(type) {
	case float64, int, int64:
		// ok
	default:
		t.Errorf("expected numeric id, got %T (%v)", rawID, rawID)
	}

	// Verify params.
	paramsRaw, ok := found["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected params map, got %T", found["params"])
	}
	if paramsRaw["uri"] != wantURI {
		t.Errorf("params.uri: want %q, got %v", wantURI, paramsRaw["uri"])
	}
	if paramsRaw["external"] != true {
		t.Errorf("params.external: want true, got %v", paramsRaw["external"])
	}
}

// TestSendServerRequestMonotonicIDs asserts that two consecutive
// sendServerRequest calls produce different, incrementing IDs.
func TestSendServerRequestMonotonicIDs(t *testing.T) {
	h := initHarnessWithCaps(t, map[string]interface{}{})
	defer h.close()

	// Issue two requests back-to-back.
	h.server.sendServerRequest("window/showDocument", protocol.ShowDocumentParams{
		URI:      "https://www.php.net/manual/en/function.strpos.php",
		External: true,
	})
	h.server.sendServerRequest("window/showDocument", protocol.ShowDocumentParams{
		URI:      "https://www.php.net/manual/en/function.strlen.php",
		External: true,
	})

	// Collect two messages from the wire.
	var ids []float64
	deadline := time.After(5 * time.Second)
	for len(ids) < 2 {
		select {
		case msg := <-h.responses:
			if method, _ := msg["method"].(string); method == "window/showDocument" {
				if id, ok := msg["id"].(float64); ok {
					ids = append(ids, id)
				}
			}
		case <-deadline:
			t.Fatalf("timed out collecting IDs (got %d so far)", len(ids))
		}
	}

	if ids[0] >= ids[1] {
		t.Errorf("expected monotonically increasing IDs, got %v then %v", ids[0], ids[1])
	}
}

// TestResponseDropSilent feeds a JSON-RPC response (id but no method) into
// the server and asserts the server does not crash and continues to serve
// subsequent requests normally.
func TestResponseDropSilent(t *testing.T) {
	rh := startedRawHarness(t)
	defer rh.close()

	// Simulate the client sending back a response to a server-initiated request.
	// This is what happens when window/showDocument gets a client ACK.
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"result":  nil,
	}
	body, _ := json.Marshal(response)
	rh.sendRaw([]byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)))

	// Give the server a moment to process the response message.
	time.Sleep(50 * time.Millisecond)

	// Server must still be alive and respond to normal requests.
	probeAlive(t, rh, 999)
}
