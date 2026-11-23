package lsp

import (
	"encoding/json"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func (s *Server) handlePrepareTypeHierarchy(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	req, ok := s.phpRequest(params.TextDocument.URI, params.Position)
	if !ok {
		s.sendResponse(msg.ID, nil)
		return
	}
	s.sendResponse(msg.ID, s.analyzer.PrepareTypeHierarchy(req.uri, req.source, req.pos))
}

func (s *Server) handleTypeHierarchyRelations(msg *jsonRPCMessage, related func(protocol.TypeHierarchyItem) []protocol.TypeHierarchyItem) {
	var params protocol.TypeHierarchyItemParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	s.sendResponse(msg.ID, related(params.Item))
}
