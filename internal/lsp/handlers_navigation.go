package lsp

import (
	"encoding/json"

	"github.com/Tusk-PHP/lsp/internal/composer/cardhover"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func (s *Server) handleDefinition(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
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
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
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
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
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
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
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
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	req, ok := s.phpRequest(params.TextDocument.URI, params.Position)
	if !ok {
		s.sendResponse(msg.ID, []protocol.Location{})
		return
	}
	s.sendResponse(msg.ID, req.locations(s.analyzer.FindAllReferences(req.uri, req.source, req.pos, s.openDocumentReader())))
}

func (s *Server) handleDocumentSymbol(msg *jsonRPCMessage) {
	var params struct {
		TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	}
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetDocumentSymbols(params.TextDocument.URI, source))
}

func (s *Server) handleFoldingRange(msg *jsonRPCMessage) {
	var params protocol.FoldingRangeParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetFoldingRanges(params.TextDocument.URI, source))
}

func (s *Server) handleDocumentHighlight(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
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
	var params protocol.WorkspaceSymbolParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	s.sendResponse(msg.ID, s.analyzer.GetWorkspaceSymbols(params.Query))
}
