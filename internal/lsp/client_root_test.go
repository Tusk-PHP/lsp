package lsp

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestSymlinkedRootAnswersInTheClientsURIs(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"composer.json": `{"autoload": {"psr-4": {"App\\": "src/"}}}`,
		"src/Base.php":  "<?php\nnamespace App;\nclass Base {}\n",
		"src/Child.php": "<?php\nnamespace App;\nclass Child extends Base {\n    public function run(): int\n    {\n        return $missing;\n    }\n}\n",
	}
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "app")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": symbols.PathToURI(link), "capabilities": map[string]interface{}{}, "processId": nil}))
	h.notify("initialized", map[string]interface{}{})
	<-h.server.indexed

	child := symbols.PathToURI(filepath.Join(link, "src", "Child.php"))
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": child, "languageId": "php", "version": 1, "text": files["src/Child.php"]},
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("no diagnostics for the document as the client named it")
		}
		raw, _ := json.Marshal(h.readNotification("textDocument/publishDiagnostics", 10*time.Second)["params"])
		var params struct {
			URI         string
			Diagnostics []struct{ Code string }
		}
		if err := json.Unmarshal(raw, &params); err != nil {
			t.Fatal(err)
		}
		if params.URI != child {
			t.Fatalf("diagnostics published for %s, want %s", params.URI, child)
		}
		if len(params.Diagnostics) > 0 {
			if params.Diagnostics[0].Code != "undefined-variable" {
				t.Errorf("diagnostics %+v, want undefined-variable", params.Diagnostics)
			}
			break
		}
	}

	resp := h.readResponse(h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": child},
		"position":     map[string]interface{}{"line": 2, "character": strings.Index("class Child extends Base {", "Base") + 1},
	}))
	raw, _ := json.Marshal(resp["result"])
	var locations []struct{ URI string }
	if err := json.Unmarshal(raw, &locations); err != nil {
		var one struct{ URI string }
		if err := json.Unmarshal(raw, &one); err != nil {
			t.Fatalf("definition result %s: %v", raw, err)
		}
		locations = append(locations, one)
	}
	want := symbols.PathToURI(filepath.Join(link, "src", "Base.php"))
	if len(locations) != 1 || locations[0].URI != want {
		t.Errorf("definition of Base: %s, want %s", raw, want)
	}
}

func TestRespellRootTouchesOnlyWholeURIs(t *testing.T) {
	r := newRootURIs("file:///link/app/", "file:///real/app")
	in := `{"uri":"file:///link/app/src/A.php","root":"file:///link/app","other":"file:///link/apple/A.php","text":"say \"file:///link/app/x\"","key":{"file:///link/app/B.php":1}}`
	want := `{"uri":"file:///real/app/src/A.php","root":"file:///real/app","other":"file:///link/apple/A.php","text":"say \"file:///link/app/x\"","key":{"file:///real/app/B.php":1}}`
	if got := string(r.toServer([]byte(in))); got != want {
		t.Errorf("to the server:\n%s\nwant\n%s", got, want)
	}
	if got := string(r.toClient([]byte(want))); got != in {
		t.Errorf("back to the client:\n%s\nwant\n%s", got, in)
	}
	if newRootURIs("file:///same", "file:///same/") != nil {
		t.Error("the same root needs no respelling")
	}
}

func TestAWorkspaceRootThatCantBeOpenedIsKeptAndReported(t *testing.T) {
	var out bytes.Buffer
	s := NewServer(strings.NewReader(""), &out, log.New(io.Discard, "", 0))
	if got := s.resolveRoot(initializeParams{}); got != "" || out.Len() != 0 {
		t.Errorf("no root: %q, sent %s", got, out.String())
	}
	missing := filepath.Join(t.TempDir(), "gone")
	if got := s.resolveRoot(initializeParams{RootPath: missing}); got != missing {
		t.Errorf("a root that can't be opened = %q, want it kept as given", got)
	}
	if !strings.Contains(out.String(), "window/showMessage") || !strings.Contains(out.String(), "can't open the workspace root") {
		t.Errorf("the user isn't told: %s", out.String())
	}
}
