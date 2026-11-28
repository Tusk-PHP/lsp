package lsp

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func (s *Server) resolveRoot(params initializeParams) string {
	given, clientURI := params.RootPath, symbols.PathToURI(params.RootPath)
	if params.RootURI != "" {
		given, clientURI = symbols.URIToPath(params.RootURI), params.RootURI
	}
	if given == "" {
		return ""
	}
	root, err := workspace.ResolveRoot(given)
	if err != nil {
		s.logger.Printf("Workspace root: %v", err)
		s.sendNotification("window/showMessage", map[string]interface{}{
			"type":    protocol.MessageTypeWarning,
			"message": "Tusk PHP can't open the workspace root, so it knows only the open files: " + err.Error(),
		})
		return given
	}
	s.clientRoot.Store(newRootURIs(clientURI, symbols.PathToURI(root)))
	return root
}

type rootURIs struct {
	client, server []byte
}

func newRootURIs(client, server string) *rootURIs {
	client = strings.TrimSuffix(client, "/")
	server = strings.TrimSuffix(server, "/")
	if client == server {
		return nil
	}
	return &rootURIs{client: openingQuoted(client), server: openingQuoted(server)}
}

func (r *rootURIs) toServer(body []byte) []byte {
	if r == nil {
		return body
	}
	return respellRoot(body, r.client, r.server)
}

func (r *rootURIs) toClient(body []byte) []byte {
	if r == nil {
		return body
	}
	return respellRoot(body, r.server, r.client)
}

func openingQuoted(uri string) []byte {
	quoted, _ := json.Marshal(uri)
	return quoted[:len(quoted)-1]
}

func respellRoot(body, from, to []byte) []byte {
	var out []byte
	written := 0
	for at := 0; ; {
		i := bytes.Index(body[at:], from)
		if i < 0 {
			break
		}
		start, end := at+i, at+i+len(from)
		at = end
		if !opensString(body, start) || end == len(body) || (body[end] != '/' && body[end] != '"') {
			continue
		}
		if out == nil {
			out = make([]byte, 0, len(body)+len(to)-len(from)+64)
		}
		out = append(out, body[written:start]...)
		out = append(out, to...)
		written = end
	}
	if out == nil {
		return body
	}
	return append(out, body[written:]...)
}

func opensString(body []byte, quote int) bool {
	backslashes := 0
	for i := quote - 1; i >= 0 && body[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 0
}
