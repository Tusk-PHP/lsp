package lsp

import (
	"encoding/json"
	"fmt"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	"github.com/Tusk-PHP/lsp/internal/introspect"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func (s *Server) handleExecuteCommand(msg *jsonRPCMessage) {
	var params protocol.ExecuteCommandParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	switch params.Command {
	case "tuskPhpLsp.copyNamespace":
		if len(params.Arguments) > 0 {
			var uri string
			if json.Unmarshal(params.Arguments[0], &uri) == nil {
				source := s.getDocument(uri)
				ns := s.analyzer.GetFileNamespace(uri, source)
				s.sendResponse(msg.ID, ns)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	case "tuskPhpLsp.namespaceForPath":
		// Returns the expected namespace for a file path based on PSR-4 autoload
		if len(params.Arguments) > 0 {
			var uri string
			if json.Unmarshal(params.Arguments[0], &uri) == nil {
				filePath := symbols.URIToPath(uri)
				autoload := composer.GetAutoloadPaths(s.rootPath)
				ns := composer.PathToNamespace(filePath, autoload)
				s.sendResponse(msg.ID, ns)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	case "tuskPhpLsp.moveToNamespace":
		// Arguments: [uri, targetNamespace]
		if len(params.Arguments) >= 2 {
			var uri, targetNS string
			if json.Unmarshal(params.Arguments[0], &uri) == nil && json.Unmarshal(params.Arguments[1], &targetNS) == nil {
				source := s.getDocument(uri)
				autoload := composer.GetAutoloadPaths(s.rootPath)
				edit := s.analyzer.MoveToNamespace(uri, source, targetNS, autoload, s.getDocumentReader())
				s.sendResponse(msg.ID, edit)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	case "tuskPhpLsp.createObject":
		// Arguments: [dirUri, kind, name]
		if len(params.Arguments) >= 3 {
			var dirURI, kind, name string
			if json.Unmarshal(params.Arguments[0], &dirURI) == nil &&
				json.Unmarshal(params.Arguments[1], &kind) == nil &&
				json.Unmarshal(params.Arguments[2], &name) == nil {
				autoload := composer.GetAutoloadPaths(s.rootPath)
				edit := buildCreateObjectEdit(dirURI, kind, name, autoload)
				s.sendResponse(msg.ID, edit)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	case "tuskPhpLsp.copyReference":
		// Arguments: [uri (string), position ({"line":N,"character":N})]
		if len(params.Arguments) >= 2 {
			var uri string
			var position protocol.Position
			if json.Unmarshal(params.Arguments[0], &uri) == nil &&
				json.Unmarshal(params.Arguments[1], &position) == nil {
				source := s.getDocument(uri)
				variants := s.analyzer.CopyReference(uri, source, position)
				s.sendResponse(msg.ID, variants)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	case "tuskPhpLsp.debugDocument":
		if len(params.Arguments) > 0 {
			var uri string
			if json.Unmarshal(params.Arguments[0], &uri) == nil {
				source := s.getDocument(uri)

				// Determine buffer state: "live-buffer" when the document is
				// open in the editor (present in s.documents), otherwise "disk".
				s.docMu.RLock()
				_, inBuffer := s.documents[uri]
				s.docMu.RUnlock()
				bufferState := "disk"
				if inBuffer {
					bufferState = "live-buffer"
				}

				// Run native checks synchronously for a self-consistent dump.
				file := parser.ParseFile(source)
				var typeRes diagnostics.TypeResolverFunc
				var builtinRule *checks.BuiltinUnavailableRule
				if s.diag != nil {
					typeRes = s.diag.TypeResolver
					builtinRule = s.diag.BuiltinUnavailableRule
				}
				rules := diagnostics.NativeRules(diagnostics.NativeRulesOptions{
					TypeResolver:           typeRes,
					BuiltinUnavailableRule: builtinRule,
				})
				findings := checks.CheckFile(file, source, s.index, rules)

				v := introspect.ParseVerbosity(s.cfg.Introspection.Verbosity)

				var profile string
				if s.workspace != nil {
					bp := s.workspace.BuiltinProfile
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
				text := introspect.Render(introspect.Document(in), v)

				s.sendResponse(msg.ID, text)

				// Zed display path: emit the dump to the LSP log panel and
				// show a toast pointing the user there.
				s.sendNotification("window/logMessage", map[string]interface{}{
					"type":    protocol.MessageTypeInfo,
					"message": text,
				})
				s.sendNotification("window/showMessage", map[string]interface{}{
					"type":    protocol.MessageTypeInfo,
					"message": "Tusk PHP: parsed-state dump written to the language server logs.",
				})
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	default:
		s.sendError(msg.ID, -32601, fmt.Sprintf("Unknown command: %s", params.Command))
	}
}
