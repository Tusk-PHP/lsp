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

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
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
	s := NewServer(strings.NewReader(""), io.Discard, log.New(io.Discard, "", 0))
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
	s := NewServer(strings.NewReader(""), &out, log.New(io.Discard, "", 0))
	s.diag = diagnostics.NewProvider(s.index, "", "", log.New(io.Discard, "", 0), config.DefaultConfig())
	uri := "file:///a.php"
	old, current := "<?php\necho $a;\n", "<?php\necho 1;\n"
	s.documents[uri] = current

	s.publishDiagnostics(uri, old)
	if out.Len() != 0 {
		t.Fatalf("published diagnostics for an old version: %s", out.String())
	}
	s.publishDiagnostics(uri, current)
	if !strings.Contains(out.String(), "publishDiagnostics") {
		t.Fatal("didn't publish diagnostics for the current version")
	}
	out.Reset()
	delete(s.documents, uri)
	s.publishDiagnostics(uri, current)
	if out.Len() != 0 {
		t.Fatalf("published diagnostics for a closed document: %s", out.String())
	}
}
