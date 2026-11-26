package lsp

import (
	"errors"
	"os"

	"github.com/Tusk-PHP/lsp/internal/document"
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
	doc := s.openDocument(uri, params.TextDocument.Version, text)
	if !isPHPDocument(uri) {
		// Non-PHP documents (composer.json, tsconfig.json, …) are only
		// stored so providers that read source — e.g. composer-hover —
		// can serve them. We never PHP-parse or diagnose them.
		return
	}
	s.docIndex.reopened(uri)
	s.indexDocumentMaybeAsync(uri, text)
	if s.cfg.DiagnosticsEnabled {
		s.publishDiagnostics(doc)
	}
}

func (s *Server) openDocument(uri string, version int, text string) document.Snapshot {
	path := symbols.URIToPath(uri)
	unsaved := s.index.GetFileSource(symbols.PathToURI(path)) != text
	return s.docs.Open(uri, path, version, text, unsaved)
}

func (s *Server) handleDidChange(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.DidChangeTextDocumentParams](msg)
	if !ok {
		return
	}
	if len(params.ContentChanges) == 0 {
		return
	}
	uri := params.TextDocument.URI
	doc, err := s.docs.Change(uri, params.TextDocument.Version, s.encoding, documentChanges(params.ContentChanges))
	switch {
	case errors.Is(err, document.ErrDiverged):
		return
	case err != nil:
		s.logger.Printf("didChange: %v", err)
		s.sendNotification("window/showMessage", map[string]interface{}{
			"type":    protocol.MessageTypeError,
			"message": "Tusk PHP lost track of a document's text and reads it from disk until you close and reopen it: " + err.Error(),
		})
		s.showDivergedFromDisk(uri)
		return
	}
	s.documentChanged(doc)
}

func (s *Server) showDivergedFromDisk(uri string) {
	content, err := os.ReadFile(symbols.URIToPath(uri))
	if err != nil {
		s.logger.Printf("didChange: reading %s from disk while its text is unknown: %v; keeping the last text that applied", uri, err)
		return
	}
	if doc, ok := s.docs.ShowDiverged(uri, string(content)); ok {
		s.documentChanged(doc)
	}
}

func (s *Server) documentChanged(doc document.Snapshot) {
	if !isPHPDocument(doc.URI) {
		return
	}
	s.indexDocumentMaybeAsync(doc.URI, doc.Text)
	if s.cfg.DiagnosticsEnabled {
		s.scheduleDiagnostics(doc.URI)
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
	unsaved := s.docs.Close(uri)
	s.blade.forget(uri)
	s.cancelDiagnostics(uri)
	if s.analysis != nil {
		s.analysis.Closed(uri)
	}
	if s.workspace == nil || !isPHPDocument(uri) {
		return
	}
	s.docIndex.afterClose(uri, func() {
		if symbols.PathToURI(path) != uri {
			s.workspace.RemoveFile(uri)
		}
		if unsaved {
			s.queueRefresh([]string{path}, false)
		}
	})
}

func (s *Server) handleDidSave(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.DidSaveTextDocumentParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	if text, ok := s.savedText(uri, params.Text); ok {
		if !s.docs.Save(uri, text) {
			s.logger.Printf("didSave: %s is not open, so its saved text is not kept as an open document", uri)
		}
		if isPHPDocument(uri) {
			s.indexDocumentMaybeAsync(uri, text)
		}
	} else {
		s.docs.MarkSaved(uri)
	}
	if !isPHPDocument(uri) {
		return
	}
	if affectsComponents(s.rootPath, uri) {
		s.session.Components().Invalidate()
	}
	s.goSafe("container.Analyze", s.container.Analyze)
	if s.cfg.DiagnosticsEnabled {
		s.goSafe("diagnostics.RunTools", func() {
			filePath := symbols.URIToPath(uri)
			// PHPStan and Pint run on .php files only: Pint would reformat a
			// template, and PHPStan doesn't analyse .phtml unless configured to.
			if !workspace.IsTemplate(uri) {
				s.analysis.RunTools(uri, filePath)
			}
			doc, open := s.docs.Get(uri)
			if !open {
				return
			}
			s.analysis.Saved(uri, doc.Text)
			s.publishDiagnostics(doc)
		})
	}
}

func (s *Server) savedText(uri string, sent *string) (string, bool) {
	if sent != nil {
		return *sent, true
	}
	if doc, open := s.docs.Get(uri); !open || !doc.Diverged() {
		return "", false
	}
	content, err := os.ReadFile(symbols.URIToPath(uri))
	if err != nil {
		s.logger.Printf("didSave: reading %s, whose text diverged from the editor's, from disk: %v", uri, err)
		return "", false
	}
	return string(content), true
}

// getDocumentReader returns a function that reads document content by URI,
// falling back to disk if the document isn't open in the editor.
func (s *Server) getDocumentReader() func(string) string {
	return func(uri string) string {
		if source, ok := s.docs.Text(uri); ok {
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
	source, _ := s.docs.Text(uri)
	return source
}

// republishOpenDocuments re-runs diagnostics for every open document.
// Called after async workspace + composer indexing settles so any findings
// that depended on a populated index (or were soft-moded during warm-up)
// produce their authoritative result.
func (s *Server) republishOpenDocuments() {
	for _, doc := range s.docs.All() {
		s.publishDiagnostics(doc)
	}
}

func documentChanges(events []protocol.TextDocumentContentChangeEvent) []document.Change {
	changes := make([]document.Change, len(events))
	for i, e := range events {
		changes[i].Text = e.Text
		if r := e.Range; r != nil {
			changes[i].Range = &document.Range{
				Start: document.Position{Line: r.Start.Line, Character: r.Start.Character},
				End:   document.Position{Line: r.End.Line, Character: r.End.Character},
			}
		}
	}
	return changes
}
