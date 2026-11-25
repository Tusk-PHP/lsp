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
	s.sendResponse(msg.ID, s.analyzer.DocumentLinks(uri, s.getDocument(uri), s.rootPath, fileExists))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
