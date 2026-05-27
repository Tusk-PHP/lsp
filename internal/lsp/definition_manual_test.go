package lsp

import (
	"strings"
	"testing"
	"time"
)

// initHarnessWithOpts creates an initialized harness using the given capabilities
// and initializationOptions maps.
func initHarnessWithOpts(t *testing.T, caps map[string]interface{}, opts map[string]interface{}) *lspHarness {
	t.Helper()
	h := newHarness(t)
	root := testdataPath()
	params := map[string]interface{}{
		"rootUri":   "file://" + root,
		"processId": nil,
	}
	if caps != nil {
		params["capabilities"] = caps
	} else {
		params["capabilities"] = map[string]interface{}{}
	}
	if opts != nil {
		params["initializationOptions"] = opts
	}
	initID := h.send("initialize", params)
	h.readResponse(initID)
	h.notify("initialized", map[string]interface{}{})
	return h
}

// openBuiltinDoc opens a document containing a call to array_map and returns
// the URI and source text.
func openBuiltinDoc(t *testing.T, h *lspHarness) (string, string) {
	t.Helper()
	const uri = "file:///tmp/defmanual_builtin.php"
	const source = "<?php\n$x = array_map(fn($v) => $v, []);\n"
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": "php",
			"version":    1,
			"text":       source,
		},
	})
	time.Sleep(50 * time.Millisecond)
	return uri, source
}

// openProjectDoc opens a document containing a call to app_version (project-defined).
func openProjectDoc(t *testing.T, h *lspHarness) (string, string) {
	t.Helper()
	const uri = "file:///tmp/defmanual_project.php"
	const source = "<?php\napp_version();\n"
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": "php",
			"version":    1,
			"text":       source,
		},
	})
	time.Sleep(50 * time.Millisecond)
	return uri, source
}

// posOfString finds the first occurrence of needle in source and returns
// (line, character) as a position map.
func posOfString(source, needle string) map[string]interface{} {
	for i, line := range strings.Split(source, "\n") {
		col := strings.Index(line, needle)
		if col >= 0 {
			return map[string]interface{}{"line": i, "character": col}
		}
	}
	return map[string]interface{}{"line": 0, "character": 0}
}

// drainMessages reads all pending messages from both channels for up to
// timeout, separating definition responses (matched by id) from
// window/showDocument server requests.
func drainMessages(h *lspHarness, defID int, timeout time.Duration) (defResp map[string]interface{}, showDocReq map[string]interface{}) {
	deadline := time.After(timeout)
	// We need the definition response before we can stop. Collect until we
	// have it, then do a short additional drain for window/showDocument.
	for defResp == nil {
		select {
		case msg := <-h.responses:
			method, _ := msg["method"].(string)
			if method == "window/showDocument" {
				showDocReq = msg
				continue
			}
			// Check if this is the definition response by matching the id.
			if rid, ok := msg["id"]; ok {
				var gotID int
				switch v := rid.(type) {
				case float64:
					gotID = int(v)
				case int:
					gotID = v
				}
				if gotID == defID {
					defResp = msg
				}
			}
		case <-deadline:
			return
		}
	}
	// Short additional drain to catch window/showDocument if it arrives
	// just after the definition response.
	if showDocReq == nil {
		extra := time.After(300 * time.Millisecond)
		for {
			select {
			case msg := <-h.responses:
				method, _ := msg["method"].(string)
				if method == "window/showDocument" {
					showDocReq = msg
					return
				}
			case <-extra:
				return
			}
		}
	}
	return
}

// ---------------------------------------------------------------------------
// Test 1: default behavior — no capability, no flag → definition returns null,
// no window/showDocument sent.
// ---------------------------------------------------------------------------

func TestDefinitionManual_DefaultNoCapNoFlag(t *testing.T) {
	// No showDocument capability, no PHPManualOpenOnDefinition flag.
	h := initHarnessWithOpts(t, map[string]interface{}{}, nil)
	defer h.close()

	uri, source := openBuiltinDoc(t, h)

	defID := h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     posOfString(source, "array_map"),
	})

	defResp, showDocReq := drainMessages(h, defID, 5*time.Second)

	if defResp == nil {
		t.Fatal("timed out waiting for textDocument/definition response")
	}
	if defResp["error"] != nil {
		t.Errorf("definition response had error: %v", defResp["error"])
	}
	// Builtin definition always returns null.
	if defResp["result"] != nil {
		t.Errorf("expected null result for builtin definition (no flag), got %v", defResp["result"])
	}
	if showDocReq != nil {
		t.Error("expected no window/showDocument request, but one was sent")
	}
}

// ---------------------------------------------------------------------------
// Test 2: flag off, capability present → definition returns null, no
// window/showDocument.
// ---------------------------------------------------------------------------

func TestDefinitionManual_CapPresentFlagOff(t *testing.T) {
	caps := map[string]interface{}{
		"window": map[string]interface{}{
			"showDocument": map[string]interface{}{"support": true},
		},
	}
	// No PHPManualOpenOnDefinition in opts → defaults to false.
	h := initHarnessWithOpts(t, caps, nil)
	defer h.close()

	uri, source := openBuiltinDoc(t, h)

	defID := h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     posOfString(source, "array_map"),
	})

	defResp, showDocReq := drainMessages(h, defID, 5*time.Second)

	if defResp == nil {
		t.Fatal("timed out waiting for textDocument/definition response")
	}
	if defResp["error"] != nil {
		t.Errorf("definition response had error: %v", defResp["error"])
	}
	if defResp["result"] != nil {
		t.Errorf("expected null result for builtin definition (flag off), got %v", defResp["result"])
	}
	if showDocReq != nil {
		t.Error("expected no window/showDocument request (flag off), but one was sent")
	}
}

// ---------------------------------------------------------------------------
// Test 3: flag on, capability missing → definition returns null, no
// window/showDocument sent.
// ---------------------------------------------------------------------------

func TestDefinitionManual_FlagOnCapMissing(t *testing.T) {
	// No showDocument capability.
	opts := map[string]interface{}{
		"phpManualOpenOnDefinition": true,
	}
	h := initHarnessWithOpts(t, map[string]interface{}{}, opts)
	defer h.close()

	if h.server.clientSupportsShowDocument {
		t.Fatal("expected clientSupportsShowDocument false — test setup error")
	}

	uri, source := openBuiltinDoc(t, h)

	defID := h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     posOfString(source, "array_map"),
	})

	defResp, showDocReq := drainMessages(h, defID, 5*time.Second)

	if defResp == nil {
		t.Fatal("timed out waiting for textDocument/definition response")
	}
	if defResp["error"] != nil {
		t.Errorf("definition response had error: %v", defResp["error"])
	}
	if defResp["result"] != nil {
		t.Errorf("expected null result for builtin definition (no capability), got %v", defResp["result"])
	}
	if showDocReq != nil {
		t.Error("expected no window/showDocument request (no capability), but one was sent")
	}
}

// ---------------------------------------------------------------------------
// Test 4: flag on + capability on, builtin function → response null,
// window/showDocument sent with correct URL.
// ---------------------------------------------------------------------------

func TestDefinitionManual_FlagOnCapOn_BuiltinFunction(t *testing.T) {
	caps := map[string]interface{}{
		"window": map[string]interface{}{
			"showDocument": map[string]interface{}{"support": true},
		},
	}
	opts := map[string]interface{}{
		"phpManualOpenOnDefinition": true,
	}
	h := initHarnessWithOpts(t, caps, opts)
	defer h.close()

	uri, source := openBuiltinDoc(t, h)

	defID := h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     posOfString(source, "array_map"),
	})

	defResp, showDocReq := drainMessages(h, defID, 5*time.Second)

	if defResp == nil {
		t.Fatal("timed out waiting for textDocument/definition response")
	}
	if defResp["error"] != nil {
		t.Errorf("definition response had error: %v", defResp["error"])
	}
	if defResp["result"] != nil {
		t.Errorf("expected null result when manual URL is returned, got %v", defResp["result"])
	}
	if showDocReq == nil {
		t.Fatal("expected window/showDocument request, but none was received")
	}

	params, ok := showDocReq["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected params map in window/showDocument request, got %T", showDocReq["params"])
	}
	const wantURI = "https://www.php.net/manual/en/function.array-map.php"
	if params["uri"] != wantURI {
		t.Errorf("window/showDocument uri: want %q, got %v", wantURI, params["uri"])
	}
	if params["external"] != true {
		t.Errorf("window/showDocument external: want true, got %v", params["external"])
	}
	if params["takeFocus"] != true {
		t.Errorf("window/showDocument takeFocus: want true, got %v", params["takeFocus"])
	}
}

// ---------------------------------------------------------------------------
// Test 5: flag on + capability on, project symbol → response has a Location,
// no window/showDocument sent.
// ---------------------------------------------------------------------------

func TestDefinitionManual_FlagOnCapOn_ProjectSymbol(t *testing.T) {
	caps := map[string]interface{}{
		"window": map[string]interface{}{
			"showDocument": map[string]interface{}{"support": true},
		},
	}
	opts := map[string]interface{}{
		"phpManualOpenOnDefinition": true,
	}
	h := initHarnessWithOpts(t, caps, opts)
	defer h.close()

	// Wait for async indexing so helpers.php (which defines app_version) is indexed.
	time.Sleep(2 * time.Second)

	uri, source := openProjectDoc(t, h)

	defID := h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     posOfString(source, "app_version"),
	})

	defResp, showDocReq := drainMessages(h, defID, 5*time.Second)

	if defResp == nil {
		t.Fatal("timed out waiting for textDocument/definition response")
	}
	if defResp["error"] != nil {
		t.Errorf("definition response had error: %v", defResp["error"])
	}
	// Project symbol definition should return a Location (non-null).
	if defResp["result"] == nil {
		t.Error("expected non-null Location for project symbol definition")
	}
	if showDocReq != nil {
		t.Error("expected no window/showDocument request for project symbol, but one was sent")
	}
}

// ---------------------------------------------------------------------------
// Test 6: locale forwarding — phpManualLocale "es" should produce a URL with
// /manual/es/ in it.
// ---------------------------------------------------------------------------

func TestDefinitionManual_LocaleForwarded(t *testing.T) {
	caps := map[string]interface{}{
		"window": map[string]interface{}{
			"showDocument": map[string]interface{}{"support": true},
		},
	}
	opts := map[string]interface{}{
		"phpManualOpenOnDefinition": true,
		"phpManualLocale":           "es",
	}
	h := initHarnessWithOpts(t, caps, opts)
	defer h.close()

	uri, source := openBuiltinDoc(t, h)

	defID := h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     posOfString(source, "array_map"),
	})

	defResp, showDocReq := drainMessages(h, defID, 5*time.Second)

	if defResp == nil {
		t.Fatal("timed out waiting for textDocument/definition response")
	}
	if showDocReq == nil {
		t.Fatal("expected window/showDocument request, but none was received")
	}

	params, ok := showDocReq["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected params map in window/showDocument request, got %T", showDocReq["params"])
	}
	gotURI, _ := params["uri"].(string)
	if !strings.Contains(gotURI, "/manual/es/") {
		t.Errorf("expected URL to contain /manual/es/, got %q", gotURI)
	}
}
