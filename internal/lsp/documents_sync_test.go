package lsp

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/document"
	"github.com/Tusk-PHP/lsp/internal/session"
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
	stale := s.docs.Open(uri, 1, text)
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
	reopened := s.docs.Open(uri, 3, text)
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

	s.docs.Open("file:///open.json", 4, "{}")
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
