package lsp

import (
	"sync"
	"time"
)

// defaultDiagnosticsDelay is how long diagnostics wait after the last edit
// of a document. Typing resets the wait, so a burst of keystrokes is
// analyzed once, after it ends.
const defaultDiagnosticsDelay = 200 * time.Millisecond

// diagDebouncer holds one pending diagnostics run per open document. The
// analysis grows with the size of the file, so it never runs on the message
// loop: a keystroke must not hold up the completion, hover or signature-help
// request queued behind it.
type diagDebouncer struct {
	mu     sync.Mutex
	timers map[string]*time.Timer // URI → pending run
	delay  time.Duration          // zero means defaultDiagnosticsDelay
}

// scheduleDiagnostics publishes diagnostics for uri in the background once
// it has gone unedited for the debounce delay. The run analyzes whatever
// text the document has by then, and publishDiagnostics drops the result if
// the document changed again while it was being worked out.
func (s *Server) scheduleDiagnostics(uri string) {
	d := &s.diagDebounce
	d.mu.Lock()
	defer d.mu.Unlock()
	if t := d.timers[uri]; t != nil {
		t.Stop()
	}
	if d.timers == nil {
		d.timers = make(map[string]*time.Timer)
	}
	delay := d.delay
	if delay <= 0 {
		delay = defaultDiagnosticsDelay
	}
	var t *time.Timer
	t = time.AfterFunc(delay, func() {
		d.mu.Lock()
		if d.timers[uri] != t {
			// Rescheduled or cancelled after this timer fired.
			d.mu.Unlock()
			return
		}
		delete(d.timers, uri)
		d.mu.Unlock()
		s.goSafe("diagnostics:"+uri, func() {
			s.docMu.RLock()
			source, open := s.documents[uri]
			s.docMu.RUnlock()
			if open {
				s.publishDiagnostics(uri, source)
			}
		})
	})
	d.timers[uri] = t
}

// cancelDiagnostics drops the pending diagnostics run of uri, if any.
func (s *Server) cancelDiagnostics(uri string) {
	d := &s.diagDebounce
	d.mu.Lock()
	defer d.mu.Unlock()
	if t := d.timers[uri]; t != nil {
		t.Stop()
		delete(d.timers, uri)
	}
}

// cancelAllDiagnostics drops every pending diagnostics run.
func (s *Server) cancelAllDiagnostics() {
	d := &s.diagDebounce
	d.mu.Lock()
	defer d.mu.Unlock()
	for uri, t := range d.timers {
		t.Stop()
		delete(d.timers, uri)
	}
}
