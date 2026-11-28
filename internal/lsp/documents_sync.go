package lsp

import (
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/session"
)

// Background work on open documents has to keep up with edits: a result
// worked out from an older version of a document must never replace one
// from a newer version. This file holds the two places where that can
// happen, indexing large documents and publishing diagnostics.

// docIndexQueue indexes documents in the background, the latest version of
// each one last. While a document is being indexed in the background, its
// newer versions wait here (only the newest is kept) instead of being
// indexed straight away, where they could finish before the older one.
type docIndexQueue struct {
	mu      sync.Mutex
	pending map[string]string // URI → newest source waiting to be indexed
	running map[string]bool   // URI → a background indexer is working on it
	closed  map[string]func() // URI → closed meanwhile: what to do once its indexer is done
}

// indexDocumentMaybeAsync indexes source synchronously for small documents
// (line count <= largeDocThreshold) so that ordinary editor interactions
// remain deterministic. Large documents are indexed in the background so the
// message loop isn't stalled, and so is every version of a document while a
// background index of it is still running, so versions land in order.
func (s *Server) indexDocumentMaybeAsync(uri, source string) {
	q := &s.docIndex
	q.mu.Lock()
	if !q.running[uri] && strings.Count(source, "\n") <= largeDocThreshold {
		q.mu.Unlock()
		s.workspace.IndexFile(uri, source)
		return
	}
	start := q.queueLocked(uri, source)
	q.mu.Unlock()
	if start {
		s.goSafe("indexFileByURI:"+uri, func() { s.drainDocIndex(uri) })
	}
}

func (s *Server) reindexOpenDocument(uri string) {
	q := &s.docIndex
	q.mu.Lock()
	source, open := s.openDocument(uri)
	start := open && q.queueLocked(uri, source)
	q.mu.Unlock()
	if start {
		s.goSafe("indexFileByURI:"+uri, func() { s.drainDocIndex(uri) })
	}
}

func (q *docIndexQueue) queueLocked(uri, source string) bool {
	if q.pending == nil {
		q.pending = make(map[string]string)
		q.running = make(map[string]bool)
	}
	q.pending[uri] = source
	start := !q.running[uri]
	q.running[uri] = true
	return start
}

// drainDocIndex indexes the newest pending version of uri until none is left.
func (s *Server) drainDocIndex(uri string) {
	q := &s.docIndex
	for {
		q.mu.Lock()
		source, ok := q.pending[uri]
		if !ok {
			closed := q.doneLocked(uri)
			q.mu.Unlock()
			if closed != nil {
				closed()
			}
			return
		}
		delete(q.pending, uri)
		q.mu.Unlock()
		s.indexDocumentVersion(uri, source)
	}
}

func (s *Server) indexDocumentVersion(uri, source string) {
	defer s.recoverPanic("indexFileByURI:" + uri)
	s.workspace.IndexFile(uri, source)
}

func (q *docIndexQueue) doneLocked(uri string) func() {
	delete(q.running, uri)
	closed := q.closed[uri]
	delete(q.closed, uri)
	return closed
}

func (q *docIndexQueue) afterClose(uri string, then func()) {
	q.mu.Lock()
	delete(q.pending, uri)
	if q.running[uri] {
		if q.closed == nil {
			q.closed = make(map[string]func())
		}
		q.closed[uri] = then
		q.mu.Unlock()
		return
	}
	q.mu.Unlock()
	then()
}

func (q *docIndexQueue) reopened(uri string) {
	q.mu.Lock()
	delete(q.closed, uri)
	q.mu.Unlock()
}

// docIndexIdle reports whether no background indexing of uri is running or
// waiting.
func (s *Server) docIndexIdle(uri string) bool {
	q := &s.docIndex
	q.mu.Lock()
	defer q.mu.Unlock()
	return !q.running[uri]
}

// publishDiagnostics analyzes source and sends its diagnostics, unless the
// document has changed or closed meanwhile. Analysis can run in the
// background (after indexing settles, after PHPStan and Pint on save), and
// the message loop publishes for every edit; publishMu makes the check and
// the send one step, so an older version's diagnostics never arrive after a
// newer version's.
func (s *Server) publishDiagnostics(uri, source string) {
	diagnostics := []protocol.Diagnostic{}
	if s.config().DiagnosticsEnabled {
		diagnostics = session.LSPDiagnostics(s.analysis.Analyze(uri, source))
	}
	diagnostics = s.positionsOf(uri, source).diagnosticsOut(uri, diagnostics)
	s.publishMu.Lock()
	defer s.publishMu.Unlock()
	if !s.isCurrentDocument(uri, source) {
		return
	}
	s.sendNotification("textDocument/publishDiagnostics", map[string]interface{}{"uri": uri, "diagnostics": diagnostics})
}

// isCurrentDocument reports whether uri is open with exactly this source.
func (s *Server) isCurrentDocument(uri, source string) bool {
	s.docMu.RLock()
	current, open := s.documents[uri]
	s.docMu.RUnlock()
	return open && current == source
}
