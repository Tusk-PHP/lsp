package lsp

import (
	"encoding/json"
	"os"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func (s *Server) handleDocumentLink(msg *jsonRPCMessage) {
	var params protocol.DocumentLinkParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
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
