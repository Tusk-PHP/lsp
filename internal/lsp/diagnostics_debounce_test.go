package lsp

import (
	"encoding/json"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	"github.com/Tusk-PHP/lsp/internal/parser"
)

// lockedBuffer collects the server's output from several goroutines.
type lockedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// slowDiagnosticsServer returns a server whose diagnostics pass blocks in
// its type resolver until release is closed, and records each source it
// analyzed.
func slowDiagnosticsServer(t *testing.T) (s *Server, out *lockedBuffer, release chan struct{}, analyzed func() []string) {
	t.Helper()
	out = &lockedBuffer{}
	s = NewServer(strings.NewReader(""), out, log.New(io.Discard, "", 0))
	s.diag = diagnostics.NewProvider(s.index, "", "", log.New(io.Discard, "", 0), config.DefaultConfig())
	s.diagDebounce.delay = 20 * time.Millisecond
	release = make(chan struct{})
	var mu sync.Mutex
	seen := map[string]bool{}
	var order []string
	s.diag.TypeResolver = func(expr, source string, line, col int, file *parser.FileNode) string {
		mu.Lock()
		if !seen[source] {
			seen[source] = true
			order = append(order, source)
		}
		mu.Unlock()
		<-release
		return ""
	}
	analyzed = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), order...)
	}
	return s, out, release, analyzed
}

func didChangeMessage(t *testing.T, uri string, version int, text string) *jsonRPCMessage {
	t.Helper()
	params, err := json.Marshal(map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": version},
		"contentChanges": []map[string]any{{"text": text}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &jsonRPCMessage{JSONRPC: "2.0", Method: "textDocument/didChange", Params: params}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// A didChange returns without waiting for the diagnostics pass, so the
// request queued behind the keystroke is answered straight away.
func TestDidChangeDoesNotWaitForDiagnostics(t *testing.T) {
	s, out, release, analyzed := slowDiagnosticsServer(t)
	defer close(release)
	uri := "file:///a.php"
	s.documents[uri] = "<?php\n"

	done := make(chan struct{})
	go func() {
		s.handleMessage(didChangeMessage(t, uri, 2, "<?php\n$a = new Foo();\n$a->bar();\n"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("didChange waited for the diagnostics pass")
	}
	// The pass still runs, in the background, and is still in progress.
	waitFor(t, "the diagnostics pass to start", func() bool { return len(analyzed()) == 1 })
	if strings.Contains(out.String(), "publishDiagnostics") {
		t.Fatal("published diagnostics before the pass finished")
	}
}

// A burst of edits is analyzed once, for its last version, and a pass that
// was overtaken by a newer edit publishes nothing.
func TestDebouncedDiagnosticsPublishOnlyTheLatestVersion(t *testing.T) {
	s, out, release, analyzed := slowDiagnosticsServer(t)
	uri := "file:///a.php"
	s.documents[uri] = "<?php\n"
	version := func(n int) string {
		return "<?php\n$a = new Foo();\n$a->bar();\n" + strings.Repeat("// edit\n", n)
	}

	// A burst of keystrokes inside the debounce window: one pass.
	burst := make(chan struct{})
	go func() {
		for v := 1; v <= 5; v++ {
			s.handleMessage(didChangeMessage(t, uri, v, version(v)))
		}
		close(burst)
	}()
	select {
	case <-burst:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("didChange waited for the diagnostics pass")
	}
	waitFor(t, "the first pass", func() bool { return len(analyzed()) == 1 })
	if got := analyzed()[0]; got != version(5) {
		t.Fatalf("analyzed %q, want the last version of the burst", got)
	}

	// Another edit while that pass is still running: its result is stale.
	latest := version(6) + "function broken($name {\n    return $name;\n}\n"
	s.handleMessage(didChangeMessage(t, uri, 6, latest))
	waitFor(t, "the second pass", func() bool { return len(analyzed()) == 2 })
	close(release)

	waitFor(t, "diagnostics to be published", func() bool { return strings.Contains(out.String(), "publishDiagnostics") })
	time.Sleep(50 * time.Millisecond) // the stale pass, if it were published, lands by now
	if n := strings.Count(out.String(), "publishDiagnostics"); n != 1 {
		t.Fatalf("published %d times, want once for the latest version:\n%s", n, out.String())
	}
	if !strings.Contains(out.String(), "syntax-error") {
		t.Fatalf("published diagnostics aren't the latest version's:\n%s", out.String())
	}
}

// Closing a document drops its pending diagnostics run.
func TestDidCloseCancelsPendingDiagnostics(t *testing.T) {
	s, out, release, analyzed := slowDiagnosticsServer(t)
	defer close(release)
	s.diagDebounce.delay = 50 * time.Millisecond
	uri := "file:///a.php"
	s.documents[uri] = "<?php\n"
	s.handleMessage(didChangeMessage(t, uri, 2, "<?php\n$a = new Foo();\n$a->bar();\n"))
	params, _ := json.Marshal(map[string]any{"textDocument": map[string]any{"uri": uri}})
	s.handleMessage(&jsonRPCMessage{JSONRPC: "2.0", Method: "textDocument/didClose", Params: params})

	time.Sleep(150 * time.Millisecond)
	if len(analyzed()) != 0 || out.String() != "" {
		t.Fatalf("a closed document was analyzed or published: %v %s", analyzed(), out.String())
	}
	s.diagDebounce.mu.Lock()
	pending := len(s.diagDebounce.timers)
	s.diagDebounce.mu.Unlock()
	if pending != 0 {
		t.Fatalf("%d diagnostics timers left after didClose", pending)
	}
}
