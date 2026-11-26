package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const checkoutSource = "<?php\nnamespace App;\nclass Checkout {\n    public function run() { new Order(); new Missing(); }\n}\n"

func TestAClientsOwnSpellingOfAURIFindsTheIndexedFileAndComesBack(t *testing.T) {
	root := filepath.Join(t.TempDir(), "team@shop")
	for name, content := range map[string]string{
		"composer.json":    `{"autoload": {"psr-4": {"App\\": "app/"}}}`,
		"app/Order.php":    "<?php\nnamespace App;\nclass Order {}\n",
		"app/Checkout.php": checkoutSource,
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t)
	t.Cleanup(h.close)
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": symbols.PathToURI(root), "processId": nil, "capabilities": map[string]interface{}{}}))
	h.notify("initialized", map[string]interface{}{})
	deadline := time.Now().Add(10 * time.Second)
	for !h.server.index.Ready() {
		if time.Now().After(deadline) {
			t.Fatal("the workspace is indexed")
		}
		time.Sleep(20 * time.Millisecond)
	}

	canonical := symbols.PathToURI(filepath.Join(root, "app/Checkout.php"))
	client := strings.Replace(canonical, "team@shop", "team%40shop", 1)
	if client == canonical {
		t.Fatal("the client spells the URI differently")
	}
	h.notify("textDocument/didOpen", map[string]interface{}{"textDocument": map[string]interface{}{
		"uri": client, "languageId": "php", "version": 1, "text": checkoutSource,
	}})

	diagnostics := h.readNotification("textDocument/publishDiagnostics", 10*time.Second)["params"].(map[string]interface{})
	if diagnostics["uri"] != client {
		t.Errorf("diagnostics are published under the client's spelling %s, got %v", client, diagnostics["uri"])
	}

	if got := definitionURI(t, h, client, positionOf(t, checkoutSource, "Order()", 1)); got != symbols.PathToURI(filepath.Join(root, "app/Order.php")) {
		t.Errorf("a request with the client's spelling resolves through the indexed files: %q", got)
	}

	var indexed []string
	for _, uri := range h.server.index.GetAllFileURIs() {
		if strings.HasSuffix(uri, "/app/Checkout.php") {
			indexed = append(indexed, uri)
		}
	}
	if len(indexed) != 1 || indexed[0] != canonical {
		t.Errorf("the open file is indexed once, under its canonical URI %s: %v", canonical, indexed)
	}

	raw, _ := json.Marshal(h.readResponse(h.send("workspace/symbol", map[string]interface{}{"query": "Checkout"}))["result"])
	var found []struct {
		Name     string
		Location struct{ URI string }
	}
	if err := json.Unmarshal(raw, &found); err != nil {
		t.Fatal(err)
	}
	var uris []string
	for _, f := range found {
		if f.Name == "Checkout" {
			uris = append(uris, f.Location.URI)
		}
	}
	if len(uris) != 1 || uris[0] != client {
		t.Errorf("the open file is indexed once and named with the client's spelling: %v", uris)
	}

	h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": client}})
	raw, _ = json.Marshal(h.readResponse(h.send("workspace/symbol", map[string]interface{}{"query": "Checkout"}))["result"])
	if !strings.Contains(string(raw), canonical) {
		t.Errorf("after closing, the file goes back to its canonical URI: %s", raw)
	}
}

func TestRespellReplacesOnlyWholeURIs(t *testing.T) {
	var c clientURIs
	c.remember([]respelledURI{{client: "file:///c%3A/p/a.php", canonical: "file:///C:/p/a.php"}})
	body := []byte(`{"uri":"file:///C:/p/a.php","other":"file:///C:/p/a.php.bak","changes":{"file:///C:/p/a.php":[]}}`)
	want := `{"uri":"file:///c%3A/p/a.php","other":"file:///C:/p/a.php.bak","changes":{"file:///c%3A/p/a.php":[]}}`
	if got := string(c.respell(body)); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	c.forget("file:///C:/p/a.php")
	if got := string(c.respell(body)); got != string(body) {
		t.Errorf("a forgotten document is sent as is: %s", got)
	}
}

func TestClosingADocumentOpenedUnderAnotherSpellingLeavesOnlyTheDiskText(t *testing.T) {
	h, root := startWatchedProjectIn(t, filepath.Join(t.TempDir(), "team@shop"), map[string]string{
		"composer.json":   `{"require": {"php": "^8.1"}}`,
		"app/Invoice.php": "<?php\nnamespace App;\nclass Invoice {}\n",
	}, map[string]interface{}{}, nil)
	<-h.server.indexed
	canonical := symbols.PathToURI(filepath.Join(root, "app/Invoice.php"))
	client := strings.Replace(canonical, "team@shop", "team%40shop", 1)
	if client == canonical {
		t.Fatal("the client spells the URI differently")
	}
	openDocument(h, client, "<?php\nnamespace App;\nclass Invoice {}\nclass EditedInEditor {}\n")
	waitForSymbols(t, h, "EditedInEditor", func(names []string) bool { return len(names) == 1 })

	h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": client}})
	waitForSymbols(t, h, "EditedInEditor", func(names []string) bool { return len(names) == 0 })
	var indexed []string
	for _, uri := range h.server.index.GetAllFileURIs() {
		if strings.HasSuffix(uri, "/app/Invoice.php") {
			indexed = append(indexed, uri)
		}
	}
	if len(indexed) != 1 || indexed[0] != canonical {
		t.Errorf("the closed file is indexed once, under its canonical URI %s: %v", canonical, indexed)
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestAFileURIThatDoesNotParseIsIndexedUnderTheClientsURIUntilClosed(t *testing.T) {
	h, _ := startWatchedProject(t, map[string]string{"composer.json": `{"require": {"php": "^8.1"}}`}, map[string]interface{}{}, nil)
	<-h.server.indexed
	var logged syncBuffer
	h.server.logger.SetOutput(&logged)
	uri := "file:///Scratch%zz.php"
	indexedUnder := func(name string) []string {
		t.Helper()
		raw, _ := json.Marshal(h.readResponse(h.send("workspace/symbol", map[string]interface{}{"query": name}))["result"])
		var found []struct{ Location struct{ URI string } }
		if err := json.Unmarshal(raw, &found); err != nil {
			t.Fatal(err)
		}
		var uris []string
		for _, f := range found {
			uris = append(uris, f.Location.URI)
		}
		return uris
	}

	openDocument(h, uri, "<?php\nclass Scratchpad {}\n")
	waitForSymbols(t, h, "Scratchpad", func(names []string) bool { return len(names) == 1 })
	if got := indexedUnder("Scratchpad"); len(got) != 1 || got[0] != uri {
		t.Errorf("the opened document is indexed under the client's URI %s: %v", uri, got)
	}

	h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument":   map[string]interface{}{"uri": uri, "version": 2},
		"contentChanges": []map[string]interface{}{{"text": "<?php\nclass ScratchEdited {}\n"}},
	})
	waitForSymbols(t, h, "ScratchEdited", func(names []string) bool { return len(names) == 1 })
	if got := indexedUnder("ScratchEdited"); len(got) != 1 || got[0] != uri {
		t.Errorf("the edited document stays under the client's URI %s: %v", uri, got)
	}
	var scratch []string
	for _, indexed := range h.server.index.GetAllFileURIs() {
		if strings.Contains(indexed, "Scratch") {
			scratch = append(scratch, indexed)
		}
	}
	if !slices.Equal(scratch, []string{uri}) {
		t.Errorf("the index holds the client's URI and no respelling of it: %v", scratch)
	}

	h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})
	waitForSymbols(t, h, "ScratchEdited", func(names []string) bool { return len(names) == 0 })
	if got := h.server.index.GetAllFileURIs(); slices.Contains(got, uri) {
		t.Errorf("closing the document drops it from the index: %v", got)
	}
	if n := strings.Count(logged.String(), "kept under the client's URI"); n != 1 || !strings.Contains(logged.String(), uri) {
		t.Errorf("opening the document is logged once, naming its URI: %q", logged.String())
	}
}

func TestADocumentThatIsNotAFileLeavesTheIndexWhenClosed(t *testing.T) {
	h, _ := startWatchedProject(t, map[string]string{"composer.json": `{"require": {"php": "^8.1"}}`}, map[string]interface{}{}, nil)
	<-h.server.indexed
	uri := "untitled:Scratch.php"
	openDocument(h, uri, "<?php\nclass Scratchpad {}\n")
	waitForSymbols(t, h, "Scratchpad", func(names []string) bool { return len(names) == 1 })

	h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})
	waitForSymbols(t, h, "Scratchpad", func(names []string) bool { return len(names) == 0 })
}
