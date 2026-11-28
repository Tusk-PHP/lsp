package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestInvalidProjectConfigIsReported(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(`{"diagnosticRules":{"unused-imports":false}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": symbols.PathToURI(root), "capabilities": map[string]interface{}{}, "processId": nil}))

	note := h.readNotification("window/showMessage", 5*time.Second)
	if note == nil {
		t.Fatal("a configuration that doesn't load must be reported")
	}
	params := note["params"].(map[string]interface{})
	message, _ := params["message"].(string)
	if params["type"] != float64(1) || !strings.Contains(message, ".tusk-php.json") || !strings.Contains(message, `unknown diagnostic rule "unused-imports"`) {
		t.Errorf("got %v", params)
	}
}

func TestSkippedIgnoreRuleIsReported(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(`{"diagnosticIgnore":[{"message":"("}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": symbols.PathToURI(root), "capabilities": map[string]interface{}{}, "processId": nil}))

	note := h.readNotification("window/showMessage", 5*time.Second)
	if note == nil {
		t.Fatal("a skipped rule must be reported")
	}
	params := note["params"].(map[string]interface{})
	message, _ := params["message"].(string)
	if params["type"] != float64(2) || !strings.Contains(message, `diagnosticIgnore message "("`) {
		t.Errorf("got %v", params)
	}
}
