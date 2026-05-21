package lsp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func laravelLSPTestdataPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "laravel")
}

func TestLaravelViewCompletionAndDefinition(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	root := laravelLSPTestdataPath()
	initID := h.send("initialize", map[string]interface{}{
		"rootUri":      "file://" + root,
		"capabilities": map[string]interface{}{},
		"processId":    nil,
	})
	h.readResponse(initID)
	h.notify("initialized", map[string]interface{}{})
	time.Sleep(500 * time.Millisecond)

	uri := "file://" + filepath.ToSlash(filepath.Join(root, "app", "Providers", "FortifyServiceProvider.php"))
	sourceBytes, err := os.ReadFile(filepath.Join(root, "app", "Providers", "FortifyServiceProvider.php"))
	if err != nil {
		t.Fatalf("failed to read Laravel provider fixture: %v", err)
	}
	source := string(sourceBytes)

	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	lines := strings.Split(source, "\n")
	line := -1
	char := -1
	for i, text := range lines {
		idx := strings.Index(text, "pages::auth.login")
		if idx >= 0 {
			line = i
			char = idx + len("pages::auth.lo")
			break
		}
	}
	if line < 0 {
		t.Fatal("failed to find Laravel view fixture line")
	}

	completeID := h.send("textDocument/completion", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": char},
	})
	completeResp := h.readResponse(completeID)
	if completeResp["error"] != nil {
		t.Fatalf("completion returned error: %v", completeResp["error"])
	}

	results, ok := completeResp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected completion array, got %T", completeResp["result"])
	}
	found := false
	for _, raw := range results {
		item, _ := raw.(map[string]interface{})
		if label, _ := item["label"].(string); label == "pages::auth.login" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pages::auth.login completion, got %#v", completeResp["result"])
	}

	definitionID := h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": char},
	})
	definitionResp := h.readResponse(definitionID)
	if definitionResp["error"] != nil {
		t.Fatalf("definition returned error: %v", definitionResp["error"])
	}

	location, ok := definitionResp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected definition object, got %T", definitionResp["result"])
	}
	gotURI, _ := location["uri"].(string)
	if !strings.Contains(gotURI, "/resources/views/pages/auth/login.blade.php") {
		t.Fatalf("expected login view definition URI, got %s", gotURI)
	}
}
