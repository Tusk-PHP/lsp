package lsp

import (
	"os"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func (s *Server) handleDocumentLink(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.DocumentLinkParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	if workspace.IsBlade(uri) {
		s.sendResponse(msg.ID, []protocol.DocumentLink{})
		return
	}
	source := s.getDocument(uri)
	s.sendResponse(msg.ID, s.positionsOf(uri, source).documentLinksOut(uri, s.analyzer.DocumentLinks(uri, source, s.rootPath, fileExists)))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
