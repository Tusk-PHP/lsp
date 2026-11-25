package lsp

import (
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func (s *Server) handlePrepareTypeHierarchy(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.TextDocumentPositionParams](msg)
	if !ok {
		return
	}
	p := s.positions()
	req, ok := s.phpRequest(params.TextDocument.URI, p.posIn(params.TextDocument.URI, params.Position))
	if !ok {
		s.sendResponse(msg.ID, nil)
		return
	}
	s.sendResponse(msg.ID, p.typeHierarchyOut(s.analyzer.PrepareTypeHierarchy(req.uri, req.source, req.pos)))
}

func (s *Server) handleTypeHierarchyRelations(msg *jsonRPCMessage, related func(protocol.TypeHierarchyItem) []protocol.TypeHierarchyItem) {
	params, ok := s.decodeParams[protocol.TypeHierarchyItemParams](msg)
	if !ok {
		return
	}
	s.sendResponse(msg.ID, s.positions().typeHierarchyOut(related(params.Item)))
}
