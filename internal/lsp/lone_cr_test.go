package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const loneCRShop = "<?php\rnamespace App;\r// Größe 😀\rclass Shop\r{\r    public function total(): int { return 1; }\r}\r"

const loneCRUse = "<?php\nuse App\\Shop;\n$shop = new Shop(); $shop->total();\n"

const loneCRScript = "<?php\r// é\rmissingFn();\r"

func TestLoneCRLineEndings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		offered []string
	}{{"default", nil}, {"utf-16 only", []string{"utf-16"}}, {"utf-8 offered", []string{"utf-16", "utf-8"}}} {
		offered := tc.offered
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			shopPath := filepath.Join(root, "src", "Shop.php")
			if err := os.MkdirAll(filepath.Dir(shopPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(shopPath, []byte(loneCRShop), 0o644); err != nil {
				t.Fatal(err)
			}
			h := newHarness(t)
			t.Cleanup(h.close)
			caps := map[string]interface{}{}
			if offered != nil {
				caps["general"] = map[string]interface{}{"positionEncodings": offered}
			}
			h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": symbols.PathToURI(root), "capabilities": caps, "processId": nil}))
			h.notify("initialized", map[string]interface{}{})
			waitFor(t, "the index to be ready", h.server.index.Ready)
			shop := symbols.PathToURI(shopPath)
			use := symbols.PathToURI(filepath.Join(root, "src", "use.php"))
			script := symbols.PathToURI(filepath.Join(root, "src", "script.php"))
			for uri, text := range map[string]string{use: loneCRUse, script: loneCRScript} {
				h.notify("textDocument/didOpen", map[string]interface{}{
					"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": text},
				})
			}
			request := func(method string, params map[string]interface{}) interface{} {
				t.Helper()
				return h.readResponse(h.send(method, params))["result"]
			}

			loc, _ := request("textDocument/definition", map[string]interface{}{
				"textDocument": map[string]interface{}{"uri": use},
				"position":     map[string]interface{}{"line": 2, "character": strings.Index("$shop = new Shop(); $shop->total();", "total") + 1},
			}).(map[string]interface{})
			if got := startOf(loc["range"]); loc["uri"] != shop || got != [2]float64{5, 20} {
				t.Errorf("definition of total() in Shop.php, whose lines end with a lone CR: %v, want 5:20", loc)
			}

			h.notify("textDocument/didOpen", map[string]interface{}{
				"textDocument": map[string]interface{}{"uri": shop, "languageId": "php", "version": 1, "text": loneCRShop},
			})
			symbolsOf, _ := request("textDocument/documentSymbol", map[string]interface{}{"textDocument": map[string]interface{}{"uri": shop}}).([]interface{})
			if len(symbolsOf) != 1 {
				t.Fatalf("document symbols of Shop.php: %v", symbolsOf)
			}
			class := symbolsOf[0].(map[string]interface{})
			method := class["children"].([]interface{})[0].(map[string]interface{})
			if got := startOf(class["range"]); got != [2]float64{3, 0} {
				t.Errorf("class Shop at %v, want 3:0", got)
			}
			if got := startOf(method["range"]); got != [2]float64{5, 0} {
				t.Errorf("method total at %v, want 5:0", got)
			}

			hover, _ := request("textDocument/hover", map[string]interface{}{
				"textDocument": map[string]interface{}{"uri": shop},
				"position":     map[string]interface{}{"line": 5, "character": 22},
			}).(map[string]interface{})
			if contents, _ := hover["contents"].(map[string]interface{}); contents == nil || !strings.Contains(contents["value"].(string), "total") {
				t.Errorf("hover on total at 5:22 of Shop.php: %v", hover)
			}

			h.notify("textDocument/didSave", map[string]interface{}{"textDocument": map[string]interface{}{"uri": script}})
			if got := unknownFunctionAt(t, h, script); got != [2]float64{2, 0} {
				t.Errorf("unknown-function at %v, want 2:0", got)
			}

			h.notify("textDocument/didChange", map[string]interface{}{
				"textDocument": map[string]interface{}{"uri": script, "version": 2},
				"contentChanges": []interface{}{map[string]interface{}{
					"range": map[string]interface{}{"start": map[string]interface{}{"line": 1, "character": 2}, "end": map[string]interface{}{"line": 2, "character": 0}},
					"text":  " ok\r",
				}},
			})
			request("textDocument/hover", map[string]interface{}{
				"textDocument": map[string]interface{}{"uri": script},
				"position":     map[string]interface{}{"line": 0, "character": 0},
			})
			if text, _ := h.server.docs.Text(script); text != "<?php\n// ok\nmissingFn();\n" {
				t.Errorf("after an edit spanning a lone CR the server has %q", text)
			}
		})
	}
}

func startOf(r interface{}) [2]float64 {
	rng, _ := r.(map[string]interface{})
	start, _ := rng["start"].(map[string]interface{})
	return [2]float64{toFloat(start["line"]), toFloat(start["character"])}
}

func unknownFunctionAt(t *testing.T, h *lspHarness, uri string) [2]float64 {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		params, _ := h.readNotification("textDocument/publishDiagnostics", 10*time.Second)["params"].(map[string]interface{})
		if params["uri"] != uri {
			continue
		}
		for _, d := range params["diagnostics"].([]interface{}) {
			if diag := d.(map[string]interface{}); diag["code"] == "unknown-function" {
				return startOf(diag["range"])
			}
		}
	}
	t.Error("no unknown-function diagnostic was published")
	return [2]float64{-1, -1}
}

func TestClosingADocumentWithLoneCRsOpenedAsOnDiskReadsNothingBack(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":    `{"require": {"php": "^8.1"}}`,
		"app/Shop.php":     loneCRShop,
		"app/Checkout.php": "<?php\nnamespace App;\nfunction checkout() { return new Shop(); }\n",
	}, map[string]interface{}{}, nil)
	<-h.server.indexed
	other := symbols.PathToURI(filepath.Join(root, "app", "Checkout.php"))
	openDocument(h, other, "<?php\nnamespace App;\nfunction checkout() { return new Shop(); }\n")
	diagnosticsPublishedFor(h, other, 3*watchedFilesDelay)

	uri := symbols.PathToURI(filepath.Join(root, "app", "Shop.php"))
	openDocument(h, uri, loneCRShop)
	h.notify("textDocument/didClose", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})
	if n := diagnosticsPublishedFor(h, other, 3*watchedFilesDelay); n != 0 {
		t.Errorf("closing a document opened with its disk text, lone CRs and all, re-diagnoses the open documents %d times", n)
	}
}
