package lsp

import (
	"encoding/json"
	"fmt"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/introspect"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func (s *Server) handleExecuteCommand(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.ExecuteCommandParams](msg)
	if !ok {
		return
	}
	switch params.Command {
	case protocol.CommandCopyNamespace:
		uri, ok := s.commandURI(msg, params, 0)
		if !ok {
			return
		}
		s.sendResponse(msg.ID, s.analyzer.GetFileNamespace(uri, s.getDocument(uri)))
	case protocol.CommandNamespaceForPath:
		uri, ok := s.commandURI(msg, params, 0)
		if !ok {
			return
		}
		path, err := symbols.URIToPath(uri)
		if err != nil {
			s.sendError(msg.ID, codeInvalidParams, err.Error())
			return
		}
		autoload := composer.GetAutoloadPaths(s.rootPath)
		s.sendResponse(msg.ID, composer.PathToNamespace(path, autoload))
	case protocol.CommandMoveToNamespace:
		uri, ok := s.commandURI(msg, params, 0)
		if !ok {
			return
		}
		targetNS, ok := s.commandArg[string](msg, params, 1)
		if !ok {
			return
		}
		autoload := composer.GetAutoloadPaths(s.rootPath)
		s.sendResponse(msg.ID, s.positions().workspaceEditOut(s.analyzer.MoveToNamespace(uri, s.getDocument(uri), targetNS, autoload, s.getDocumentReader())))
	case protocol.CommandCreateObject:
		dirURI, ok := s.commandURI(msg, params, 0)
		if !ok {
			return
		}
		kind, ok := s.commandArg[string](msg, params, 1)
		if !ok {
			return
		}
		name, ok := s.commandArg[string](msg, params, 2)
		if !ok {
			return
		}
		autoload := composer.GetAutoloadPaths(s.rootPath)
		s.sendResponse(msg.ID, buildCreateObjectEdit(dirURI, kind, name, autoload))
	case protocol.CommandCopyReference:
		uri, ok := s.commandURI(msg, params, 0)
		if !ok {
			return
		}
		position, ok := s.commandArg[protocol.Position](msg, params, 1)
		if !ok {
			return
		}
		source := s.getDocument(uri)
		s.sendResponse(msg.ID, s.analyzer.CopyReference(uri, source, s.positionsOf(uri, source).posIn(uri, position)))
	case protocol.CommandDebugDocument:
		uri, ok := s.commandURI(msg, params, 0)
		if !ok {
			return
		}
		text := s.debugDocument(uri)
		s.sendResponse(msg.ID, text)
		s.sendNotification("window/logMessage", map[string]interface{}{
			"type":    protocol.MessageTypeInfo,
			"message": text,
		})
		s.sendNotification("window/showMessage", map[string]interface{}{
			"type":    protocol.MessageTypeInfo,
			"message": "Tusk PHP: parsed-state dump written to the language server logs.",
		})
	default:
		s.sendError(msg.ID, codeMethodNotFound, fmt.Sprintf("Unknown command: %s", params.Command))
	}
}

func (s *Server) commandArg[T any](msg *jsonRPCMessage, params protocol.ExecuteCommandParams, i int) (T, bool) {
	var arg T
	if i >= len(params.Arguments) {
		s.rejectParams(msg, fmt.Errorf("%s takes argument %d, got %d arguments", params.Command, i+1, len(params.Arguments)))
		return arg, false
	}
	if err := json.Unmarshal(params.Arguments[i], &arg); err != nil {
		s.rejectParams(msg, fmt.Errorf("%s argument %d: %w", params.Command, i+1, err))
		return arg, false
	}
	return arg, true
}

func (s *Server) commandURI(msg *jsonRPCMessage, params protocol.ExecuteCommandParams, i int) (string, bool) {
	uri, ok := s.commandArg[string](msg, params, i)
	return symbols.CanonicalURI(uri), ok
}

func (s *Server) debugDocument(uri string) string {
	source := s.getDocument(uri)
	s.docMu.RLock()
	_, inBuffer := s.documents[uri]
	s.docMu.RUnlock()
	bufferState := "disk"
	if inBuffer {
		bufferState = "live-buffer"
	}

	var findings []checks.Finding
	if s.analysis != nil {
		findings = s.analysis.NativeFindings(source)
	}

	v := introspect.ParseVerbosity(s.cfg.Introspection.Verbosity)

	var profile string
	if s.workspace != nil {
		bp := s.workspace.Profile()
		profile = introspect.FormatProfile(bp.PHPVersion, bp.Extensions)
	}

	in := introspect.Input{
		URI:           uri,
		Source:        source,
		Index:         s.index,
		Container:     s.container,
		Framework:     s.framework,
		ServerVersion: ServerVersion,
		Profile:       profile,
		BufferState:   bufferState,
		Diagnostics:   findings,
		Verbosity:     v,
	}
	return introspect.Render(introspect.Document(in), v)
}
