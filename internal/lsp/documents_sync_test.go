package lsp

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/document"
	"github.com/Tusk-PHP/lsp/internal/session"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// choppyWriter writes each frame in small pieces, yielding in between, the
// way a pipe can take a large write. It isn't safe for concurrent use, so
// without one writer at a time the race detector or the frame check fails.
type choppyWriter struct{ buf []byte }

func (w *choppyWriter) Write(p []byte) (int, error) {
	for i := 0; i < len(p); i += 7 {
		end := min(i+7, len(p))
		w.buf = append(w.buf, p[i:end]...)
		runtime.Gosched()
	}
	return len(p), nil
}

// Messages sent from many goroutines at once arrive as whole frames.
func TestConcurrentMessagesArriveWhole(t *testing.T) {
	w := &choppyWriter{}
	s := NewServer(strings.NewReader(""), w, log.New(io.Discard, "", 0))
	const senders, each = 8, 25
	var wg sync.WaitGroup
	for g := 0; g < senders; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				s.sendNotification("window/logMessage", map[string]any{"type": 3, "message": strings.Repeat("x", 50) + strconv.Itoa(g*1000+i)})
			}
		}(g)
	}
	wg.Wait()

	rest := w.buf
	frames := 0
	for len(rest) > 0 {
		header, body, ok := bytes.Cut(rest, []byte("\r\n\r\n"))
		if !ok || !bytes.HasPrefix(header, []byte("Content-Length: ")) {
			t.Fatalf("frame %d: bad header %q", frames, header)
		}
		n, err := strconv.Atoi(string(header[len("Content-Length: "):]))
		if err != nil || n > len(body) {
			t.Fatalf("frame %d: bad length %q", frames, header)
		}
		var msg map[string]any
		if err := json.Unmarshal(body[:n], &msg); err != nil {
			t.Fatalf("frame %d: body isn't one JSON message: %v", frames, err)
		}
		rest = body[n:]
		frames++
	}
	if frames != senders*each {
		t.Fatalf("got %d frames, want %d", frames, senders*each)
	}
}

// A large document indexed in the background never overwrites a newer
// version of it that was indexed meanwhile.
func TestBackgroundIndexingKeepsTheNewestVersion(t *testing.T) {
	s := newServerWithWorkspace(t, io.Discard)
	uri := "file:///big.php"
	large := "<?php\nclass OldName {}\n" + strings.Repeat("// filler\n", largeDocThreshold+10)
	small := "<?php\nclass NewName {}\n"
	for i := 0; i < 20; i++ {
		s.indexDocumentMaybeAsync(uri, large)
		s.indexDocumentMaybeAsync(uri, small)
		deadline := time.Now().Add(5 * time.Second)
		for !s.docIndexIdle(uri) {
			if time.Now().After(deadline) {
				t.Fatal("background indexing never finished")
			}
			time.Sleep(time.Millisecond)
		}
		if s.index.Lookup("OldName") != nil || s.index.Lookup("NewName") == nil {
			t.Fatalf("round %d: the older version won: OldName=%v NewName=%v", i, s.index.Lookup("OldName") != nil, s.index.Lookup("NewName") != nil)
		}
	}
}

// Diagnostics for a version of a document that has since changed, or for a
// document that has closed, aren't published.
func TestDiagnosticsArePublishedOnlyForTheCurrentVersion(t *testing.T) {
	var out bytes.Buffer
	s := newServerWithWorkspace(t, &out)
	s.analysis = session.New(s.workspace, session.Options{})
	uri := "file:///a.php"
	text := "<?php\necho $a;\n"
	stale := s.openDocument(uri, 1, text)
	change := func(version int, to string) {
		t.Helper()
		if _, err := s.docs.Change(uri, version, document.UTF16, []document.Change{{Text: to}}); err != nil {
			t.Fatal(err)
		}
	}
	change(2, "<?php\necho 1;\n")
	change(3, text)

	s.publishDiagnostics(stale)
	if out.Len() != 0 {
		t.Fatalf("published diagnostics for version 1 after version 3, whose text is the same: %s", out.String())
	}
	current, _ := s.docs.Get(uri)
	s.publishDiagnostics(current)
	if !strings.Contains(out.String(), "publishDiagnostics") {
		t.Fatal("didn't publish diagnostics for the current version")
	}
	out.Reset()
	s.docs.Close(uri)
	s.publishDiagnostics(current)
	if out.Len() != 0 {
		t.Fatalf("published diagnostics for a closed document: %s", out.String())
	}
	reopened := s.openDocument(uri, 3, text)
	s.publishDiagnostics(current)
	if out.Len() != 0 {
		t.Fatalf("published diagnostics from before the document was closed and reopened with the same version and text: %s", out.String())
	}
	s.publishDiagnostics(reopened)
	if !strings.Contains(out.String(), "publishDiagnostics") {
		t.Fatal("didn't publish diagnostics for the reopened document")
	}
}

func TestSavedTextOfADocumentThatIsNotOpenIsNotKept(t *testing.T) {
	var logs bytes.Buffer
	s := NewServer(strings.NewReader(""), io.Discard, log.New(&logs, "", 0))
	s.handleMessage(saveMessage(t, "file:///closed.json", "{}"))
	if _, open := s.docs.Get("file:///closed.json"); open {
		t.Error("a save of a document that is not open opened it")
	}
	if !strings.Contains(logs.String(), "file:///closed.json is not open") {
		t.Errorf("the ignored save was not logged: %q", logs.String())
	}

	s.openDocument("file:///open.json", 4, "{}")
	before, _ := s.docs.Get("file:///open.json")
	s.handleMessage(saveMessage(t, "file:///open.json", "{}"))
	if after, _ := s.docs.Get("file:///open.json"); !s.docs.Current(before) || after.Version != 4 {
		t.Errorf("saving the text a document already has changed it: %+v", after)
	}
	s.handleMessage(saveMessage(t, "file:///open.json", `{"a": 1}`))
	if after, _ := s.docs.Get("file:///open.json"); s.docs.Current(before) || after.Text != `{"a": 1}` || after.Version != 4 {
		t.Errorf("a save with new text: %+v", after)
	}
}

func saveMessage(t *testing.T, uri, text string) *jsonRPCMessage {
	t.Helper()
	params, err := json.Marshal(map[string]any{"textDocument": map[string]any{"uri": uri}, "text": text})
	if err != nil {
		t.Fatal(err)
	}
	return &jsonRPCMessage{JSONRPC: "2.0", Method: "textDocument/didSave", Params: params}
}

func newServerWithWorkspace(t *testing.T, out io.Writer) *Server {
	t.Helper()
	s := NewServer(strings.NewReader(""), out, log.New(io.Discard, "", 0))
	s.workspace = workspace.New(workspace.Options{RootPath: t.TempDir(), Framework: "none", Logger: log.New(io.Discard, "", 0)})
	s.index = s.workspace.Index
	return s
}

func TestARejectedChangeReadsTheFileFromDiskUntilTheFullTextArrives(t *testing.T) {
	var out bytes.Buffer
	s := NewServer(strings.NewReader(""), &out, log.New(io.Discard, "", 0))
	path := filepath.Join(t.TempDir(), "a.json")
	if err := os.WriteFile(path, []byte("{\"disk\": 1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := symbols.PathToURI(path)
	s.openDocument(uri, 1, "{\n}\n")
	text := func() string {
		t.Helper()
		doc, _ := s.docs.Get(uri)
		return doc.Text
	}

	s.handleMessage(changeMessage(t, uri, 2, map[string]any{"range": lspRange(1, 1, 1, 0), "text": "x"}))
	s.handleMessage(changeMessage(t, uri, 3, map[string]any{"range": lspRange(1, 0, 1, 0), "text": "\"a\": 1"}))
	if got := strings.Count(out.String(), "window/showMessage"); got != 1 {
		t.Errorf("showed %d messages for one divergence, want 1: %s", got, out.String())
	}
	if got := text(); got != "{\"disk\": 1}\n" {
		t.Errorf("while the text is unknown the server has %q, want the file on disk", got)
	}

	s.handleMessage(changeMessage(t, uri, 4, map[string]any{"text": "{\n}\n"}, map[string]any{"range": lspRange(1, 0, 1, 0), "text": "\"b\": 2\n"}))
	s.handleMessage(changeMessage(t, uri, 5, map[string]any{"range": lspRange(1, 0, 1, 0), "text": "\"c\": 3,\n"}))
	if got := text(); got != "{\n\"c\": 3,\n\"b\": 2\n}\n" {
		t.Errorf("after the full text and a range edit the server has %q", got)
	}
	if got := strings.Count(out.String(), "window/showMessage"); got != 1 {
		t.Errorf("showed %d messages, want still 1", got)
	}
}

func TestAChangeToADocumentThatIsNotOpenIsOnlyLogged(t *testing.T) {
	var out, logs bytes.Buffer
	s := NewServer(strings.NewReader(""), &out, log.New(&logs, "", 0))
	s.handleMessage(changeMessage(t, "file:///closed.json", 2, map[string]any{"range": lspRange(0, 0, 0, 0), "text": "x"}))
	if strings.Contains(out.String(), "window/showMessage") {
		t.Errorf("a change to a document that is not open showed a message: %s", out.String())
	}
	if !strings.Contains(logs.String(), "file:///closed.json, version 2") {
		t.Errorf("the ignored change was not logged: %q", logs.String())
	}
	if _, open := s.docs.Get("file:///closed.json"); open {
		t.Error("a change to a document that is not open opened it")
	}
}

func TestSavingADivergedDocumentShowsTheDiskButKeepsWaitingForTheFullText(t *testing.T) {
	s := NewServer(strings.NewReader(""), io.Discard, log.New(io.Discard, "", 0))
	path := filepath.Join(t.TempDir(), "a.json")
	uri := symbols.PathToURI(path)
	s.openDocument(uri, 1, "{}\n")
	s.handleMessage(changeMessage(t, uri, 2, map[string]any{"range": lspRange(-1, 0, 0, 0), "text": "x"}))
	if err := os.WriteFile(path, []byte("{\"saved\": 1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.handleMessage(changeMessage(t, uri, 3, map[string]any{"range": lspRange(0, 1, 0, 1), "text": "\"typed\": 1, "}))
	params, err := json.Marshal(map[string]any{"textDocument": map[string]any{"uri": uri}})
	if err != nil {
		t.Fatal(err)
	}
	s.handleMessage(&jsonRPCMessage{JSONRPC: "2.0", Method: "textDocument/didSave", Params: params})
	if doc, _ := s.docs.Get(uri); doc.Text != "{\"saved\": 1}\n" || !doc.Diverged() {
		t.Errorf("after a save the diverged document is %+v, want the file on disk, still diverged", doc)
	}
	s.handleMessage(changeMessage(t, uri, 4, map[string]any{"range": lspRange(0, 1, 0, 1), "text": "\"a\": 0, "}))
	if doc, _ := s.docs.Get(uri); doc.Text != "{\"saved\": 1}\n" || !doc.Diverged() {
		t.Errorf("a range edit after the save applied to text one edit behind the editor's: %+v", doc)
	}
	s.handleMessage(changeMessage(t, uri, 5, map[string]any{"text": "{\"typed\": 1, \"a\": 0, \"saved\": 1}\n"}))
	if doc, _ := s.docs.Get(uri); doc.Text != "{\"typed\": 1, \"a\": 0, \"saved\": 1}\n" || doc.Diverged() {
		t.Errorf("after the full text: %+v", doc)
	}
}

func lspRange(sl, sc, el, ec int) map[string]any {
	return map[string]any{"start": map[string]any{"line": sl, "character": sc}, "end": map[string]any{"line": el, "character": ec}}
}

func changeMessage(t *testing.T, uri string, version int, changes ...map[string]any) *jsonRPCMessage {
	t.Helper()
	params, err := json.Marshal(map[string]any{"textDocument": map[string]any{"uri": uri, "version": version}, "contentChanges": changes})
	if err != nil {
		t.Fatal(err)
	}
	return &jsonRPCMessage{JSONRPC: "2.0", Method: "textDocument/didChange", Params: params}
}

func TestAChangeWithoutEditsLeavesTheDocumentAsItWas(t *testing.T) {
	s := NewServer(strings.NewReader(""), io.Discard, log.New(io.Discard, "", 0))
	uri := "file:///a.json"
	before := s.openDocument(uri, 1, "{}\n")
	s.handleMessage(changeMessage(t, uri, 2))
	if doc, _ := s.docs.Get(uri); !s.docs.Current(before) || doc.Version != 1 {
		t.Errorf("a change without edits made a new version of the document: %+v", doc)
	}
}

func TestSavingADivergedDocumentWhoseFileIsGoneKeepsItDiverged(t *testing.T) {
	var logs bytes.Buffer
	s := NewServer(strings.NewReader(""), io.Discard, log.New(&logs, "", 0))
	uri := symbols.PathToURI(filepath.Join(t.TempDir(), "gone.json"))
	s.openDocument(uri, 1, "{}\n")
	s.handleMessage(changeMessage(t, uri, 2, map[string]any{"range": lspRange(-1, 0, 0, 0), "text": "x"}))
	params, err := json.Marshal(map[string]any{"textDocument": map[string]any{"uri": uri}})
	if err != nil {
		t.Fatal(err)
	}
	s.handleMessage(&jsonRPCMessage{JSONRPC: "2.0", Method: "textDocument/didSave", Params: params})
	if doc, _ := s.docs.Get(uri); !doc.Diverged() || doc.Text != "{}\n" {
		t.Errorf("a save whose text can't be read from disk ended the divergence: %+v", doc)
	}
	if !strings.Contains(logs.String(), "didSave: reading "+uri) {
		t.Errorf("the failed read was not logged: %q", logs.String())
	}
}

func TestSavingAPHPDocumentThatIsNotOpenPublishesNoDiagnostics(t *testing.T) {
	h := initHarness(t)
	defer h.close()
	uri := "file://" + filepath.Join(testdataPath(), "app/NeverOpenedSave.php")
	h.notify("textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"text":         "<?php\nclass NeverOpenedSave { public function f() { return $undefined; } }\n",
	})
	deadline := time.Now().Add(5 * time.Second)
	for h.server.index.Lookup("NeverOpenedSave") == nil {
		if time.Now().After(deadline) {
			t.Fatal("the saved text was never indexed")
		}
		time.Sleep(time.Millisecond)
	}
	quiet := time.After(500 * time.Millisecond)
	for {
		select {
		case note := <-h.notes:
			params, _ := note["params"].(map[string]any)
			if note["method"] == "textDocument/publishDiagnostics" && params["uri"] == uri {
				t.Fatalf("published diagnostics for a document that is not open: %v", note)
			}
		case <-quiet:
			return
		}
	}
}
