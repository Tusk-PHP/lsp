package lsp

import (
	"github.com/Tusk-PHP/lsp/internal/composer/cardhover"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func (s *Server) handleDefinition(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.TextDocumentPositionParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	source := s.getDocument(uri)

	if cardhover.IsComposerJSON(uri) {
		result := s.composerHover.Definition(uri, source, params.Position)
		if result.ExternalURL != "" && s.clientSupportsShowDocument {
			s.sendServerRequest("window/showDocument", protocol.ShowDocumentParams{
				URI:       result.ExternalURL,
				External:  true,
				TakeFocus: true,
			})
		}
		s.sendResponse(msg.ID, nil)
		return
	}

	if locs, ok := s.bladeDefinition(uri, params.Position); ok {
		s.sendResponse(msg.ID, locs)
		return
	}
	req, ok := s.phpRequest(uri, params.Position)
	if !ok {
		s.sendResponse(msg.ID, nil)
		return
	}
	if req.doc != nil {
		if locs, ok := viewVarDefinition(req.vars, req.template, req.offset); ok {
			s.sendResponse(msg.ID, locs)
			return
		}
	}
	openManual := s.cfg.PHPManualOpenOnDefinition && s.clientSupportsShowDocument
	result := s.analyzer.FindDefinitionWithManual(req.uri, req.source, req.pos, openManual, s.cfg.PHPManualLocale)

	if result.ExternalManualURL != "" {
		s.sendServerRequest("window/showDocument", protocol.ShowDocumentParams{
			URI:       result.ExternalManualURL,
			External:  true,
			TakeFocus: true,
		})
		s.sendResponse(msg.ID, nil)
		return
	}

	s.sendResponse(msg.ID, req.location(result.Location))
}

func (s *Server) handleDeclaration(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.TextDocumentPositionParams](msg)
	if !ok {
		return
	}
	if locs, ok := s.bladeDefinition(params.TextDocument.URI, params.Position); ok {
		s.sendResponse(msg.ID, locs)
		return
	}
	req, ok := s.phpRequest(params.TextDocument.URI, params.Position)
	if !ok {
		s.sendResponse(msg.ID, nil)
		return
	}
	s.sendResponse(msg.ID, req.location(s.analyzer.FindDeclaration(req.uri, req.source, req.pos)))
}

func (s *Server) handleTypeDefinition(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.TextDocumentPositionParams](msg)
	if !ok {
		return
	}
	req, ok := s.phpRequest(params.TextDocument.URI, params.Position)
	if !ok {
		s.sendResponse(msg.ID, []protocol.Location{})
		return
	}
	s.sendResponse(msg.ID, req.locations(s.analyzer.FindTypeDefinition(req.uri, req.source, req.pos)))
}

func (s *Server) handleImplementation(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.TextDocumentPositionParams](msg)
	if !ok {
		return
	}
	req, ok := s.phpRequest(params.TextDocument.URI, params.Position)
	if !ok {
		s.sendResponse(msg.ID, []protocol.Location{})
		return
	}
	s.sendResponse(msg.ID, req.locations(s.analyzer.FindImplementation(req.uri, req.source, req.pos)))
}

func (s *Server) handleReferences(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.TextDocumentPositionParams](msg)
	if !ok {
		return
	}
	req, ok := s.phpRequest(params.TextDocument.URI, params.Position)
	if !ok {
		s.sendResponse(msg.ID, []protocol.Location{})
		return
	}
	s.answerInBackground(msg, req, func() any {
		return req.locations(s.analyzer.FindAllReferences(req.uri, req.source, req.pos, s.openDocumentReader()))
	})
}

// answerInBackground answers a request whose cost grows with the workspace
// off the message loop. The answer is sent only if the document it was asked
// on is still the same text; otherwise its positions may be stale, and the
// client is told the content changed.
func (s *Server) answerInBackground(msg *jsonRPCMessage, req bladeRequest, answer func() any) {
	s.goSafe(msg.Method, func() {
		result := answer()
		if s.getDocument(req.uri) != req.text() {
			s.sendError(msg.ID, codeContentModified, msg.Method+": "+req.uri+" changed while the request ran")
			return
		}
		s.sendResponse(msg.ID, result)
	})
}

type documentSymbolParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
}

func (s *Server) handleDocumentSymbol(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[documentSymbolParams](msg)
	if !ok {
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetDocumentSymbols(params.TextDocument.URI, source))
}

func (s *Server) handleFoldingRange(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.FoldingRangeParams](msg)
	if !ok {
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetFoldingRanges(params.TextDocument.URI, source))
}

func (s *Server) handleDocumentHighlight(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.TextDocumentPositionParams](msg)
	if !ok {
		return
	}
	req, ok := s.phpRequest(params.TextDocument.URI, params.Position)
	if !ok {
		s.sendResponse(msg.ID, []protocol.DocumentHighlight{})
		return
	}
	s.sendResponse(msg.ID, req.highlights(s.analyzer.GetDocumentHighlights(req.uri, req.source, req.pos)))
}

func (s *Server) handleWorkspaceSymbol(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.WorkspaceSymbolParams](msg)
	if !ok {
		return
	}
	s.sendResponse(msg.ID, s.analyzer.GetWorkspaceSymbols(params.Query))
}
