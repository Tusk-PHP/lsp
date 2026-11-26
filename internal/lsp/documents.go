package lsp

import (
	"os"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func (s *Server) handleDidOpen(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.DidOpenTextDocumentParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	text := params.TextDocument.Text
	path := symbols.URIToPath(uri)
	unsaved := s.index.GetFileSource(symbols.PathToURI(path)) != text
	s.docMu.Lock()
	s.documents[uri] = text
	s.openPaths[path] = uri
	s.unsaved[uri] = unsaved
	s.docMu.Unlock()
	if !isPHPDocument(uri) {
		// Non-PHP documents (composer.json, tsconfig.json, …) are only
		// stored so providers that read source — e.g. composer-hover —
		// can serve them. We never PHP-parse or diagnose them.
		return
	}
	s.indexDocumentMaybeAsync(uri, text)
	if s.cfg.DiagnosticsEnabled {
		s.publishDiagnostics(uri, text)
	}
}

func (s *Server) handleDidChange(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.DidChangeTextDocumentParams](msg)
	if !ok {
		return
	}
	if len(params.ContentChanges) > 0 {
		source := params.ContentChanges[len(params.ContentChanges)-1].Text
		uri := params.TextDocument.URI
		s.docMu.Lock()
		s.documents[uri] = source
		s.unsaved[uri] = true
		s.docMu.Unlock()
		if !isPHPDocument(uri) {
			return
		}
		s.indexDocumentMaybeAsync(uri, source)
		if s.cfg.DiagnosticsEnabled {
			s.scheduleDiagnostics(uri)
		}
	}
}

// isPHPDocument reports whether the URI points at a PHP source file.
// Used to gate PHP-only paths (indexing, diagnostics) so non-PHP
// documents — composer.json, tsconfig.json, etc. — can be opened
// without polluting the symbol index or generating diagnostics.
func isPHPDocument(uri string) bool {
	return workspace.IsPHPSource(uri)
}

func (s *Server) handleDidClose(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.DidCloseTextDocumentParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	path := symbols.URIToPath(uri)
	s.docMu.Lock()
	delete(s.documents, uri)
	if s.openPaths[path] == uri {
		delete(s.openPaths, path)
	}
	unsaved := s.unsaved[uri]
	delete(s.unsaved, uri)
	s.docMu.Unlock()
	s.blade.forget(uri)
	s.cancelDiagnostics(uri)
	s.diag.ClearCache(uri)
	if s.workspace == nil || !isPHPDocument(uri) {
		return
	}
	if symbols.PathToURI(path) != uri {
		s.workspace.RemoveFile(uri)
	}
	if unsaved {
		s.queueRefresh([]string{path}, false)
	}
}

func (s *Server) handleDidSave(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.DidSaveTextDocumentParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	s.docMu.Lock()
	if _, open := s.documents[uri]; open {
		s.unsaved[uri] = false
	}
	if params.Text != nil {
		s.documents[uri] = *params.Text
	}
	s.docMu.Unlock()
	if params.Text != nil && isPHPDocument(uri) {
		s.indexDocumentMaybeAsync(uri, *params.Text)
	}
	if !isPHPDocument(uri) {
		return
	}
	if affectsComponents(s.rootPath, uri) {
		s.components.Invalidate()
	}
	s.goSafe("container.Analyze", s.container.Analyze)
	if s.cfg.DiagnosticsEnabled {
		s.goSafe("diagnostics.RunTools", func() {
			filePath := symbols.URIToPath(uri)
			// PHPStan and Pint run on .php files only: Pint would reformat a
			// template, and PHPStan doesn't analyse .phtml unless configured to.
			if !workspace.IsTemplate(uri) {
				s.diag.RunTools(uri, filePath)
			}
			source := s.getDocument(uri)
			s.diag.AnalyzeOnSave(uri, source)
			s.publishDiagnostics(uri, source)
		})
	}
}

// getDocumentReader returns a function that reads document content by URI,
// falling back to disk if the document isn't open in the editor.
func (s *Server) getDocumentReader() func(string) string {
	return func(uri string) string {
		s.docMu.RLock()
		source, ok := s.documents[uri]
		s.docMu.RUnlock()
		if ok {
			return source
		}
		// Fall back to reading from disk
		path := symbols.URIToPath(uri)
		content, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return string(content)
	}
}

// openDocumentReader reads open documents only, returning "" for the rest so
// the caller falls back to the source the index keeps.
func (s *Server) openDocumentReader() func(string) string {
	return s.getDocument
}

func (s *Server) getDocument(uri string) string {
	s.docMu.RLock()
	source := s.documents[uri]
	s.docMu.RUnlock()
	return source
}

// republishOpenDocuments re-runs diagnostics for every open document.
// Called after async workspace + composer indexing settles so any findings
// that depended on a populated index (or were soft-moded during warm-up)
// produce their authoritative result.
func (s *Server) republishOpenDocuments() {
	s.docMu.RLock()
	pairs := make([][2]string, 0, len(s.documents))
	for uri, source := range s.documents {
		pairs = append(pairs, [2]string{uri, source})
	}
	s.docMu.RUnlock()

	for _, p := range pairs {
		s.publishDiagnostics(p[0], p[1])
	}
}
