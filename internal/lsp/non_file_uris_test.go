package lsp

import (
	"path/filepath"
	"testing"
)

func TestFileFeaturesAnswerNothingForADocumentThatIsNotAFile(t *testing.T) {
	const uri = "untitled:app/Providers/AppServiceProvider.php"
	if affectsComponents("/p", uri) {
		t.Error("a document that is not a file changes no components")
	}
	if got := (&Server{rootPath: "/p"}).relativePath(uri); got != uri {
		t.Errorf("a document that is not a file is named by its URI, got %q", got)
	}
	if edit := buildCreateObjectEdit("untitled:app", "class", "Order", nil); edit != nil {
		t.Errorf("no file is created in a directory that is not a file: %+v", edit)
	}
}

func TestNamespaceForPathRejectsADocumentThatIsNotAFile(t *testing.T) {
	h, _ := startWatchedProject(t, map[string]string{"composer.json": `{"autoload": {"psr-4": {"App\\": "app/"}}}`}, map[string]interface{}{}, nil)
	resp := h.readResponse(h.send("workspace/executeCommand", map[string]interface{}{
		"command":   "tuskPhpLsp.namespaceForPath",
		"arguments": []interface{}{"untitled:Scratch.php"},
	}))
	rpcErr, _ := resp["error"].(map[string]interface{})
	if rpcErr == nil || rpcErr["code"] != float64(codeInvalidParams) {
		t.Errorf("a URI that is not a file has no namespace: %v", resp)
	}
}

func TestFileCoverageOfADocumentThatIsNotAFileIsNull(t *testing.T) {
	root := coverageProject(t)
	h, _ := coverageHarness(t, root, nil)
	defer h.close()
	h.send("tusk/coverageReport", nil)
	h.readResponse(h.nextID - 1)
	id := h.send("tusk/fileCoverage", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "untitled:" + filepath.ToSlash(filepath.Join(root, "app/Untested.php"))},
	})
	if got := h.readResponse(id)["result"]; got != nil {
		t.Errorf("a document that is not a file has no coverage, got %v", got)
	}
}
