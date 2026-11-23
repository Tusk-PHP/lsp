package lsp

import (
	"encoding/json"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/composer/cardhover"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func (s *Server) handleCompletion(msg *jsonRPCMessage) {
	var params struct {
		protocol.TextDocumentPositionParams
		Context *struct {
			TriggerCharacter string `json:"triggerCharacter"`
		} `json:"context"`
	}
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	uri := params.TextDocument.URI
	if workspace.IsBlade(uri) {
		if items, ok := s.bladeCompletion(uri, params.Position); ok {
			s.sendResponse(msg.ID, items)
			return
		}
	} else if params.Context != nil && params.Context.TriggerCharacter == "@" {
		// `@` starts a Blade directive; in PHP it has nothing to complete.
		s.sendResponse(msg.ID, []protocol.CompletionItem{})
		return
	}
	req, ok := s.phpRequest(uri, params.Position)
	if !ok {
		s.sendResponse(msg.ID, []protocol.CompletionItem{})
		return
	}
	items := req.completions(s.completion.GetCompletions(req.uri, req.source, req.pos))
	if req.doc != nil {
		items = withViewVarCompletions(items, req.vars, req.template, req.offset)
	}
	s.sendResponse(msg.ID, items)
}

// handleCompletionResolve fills in the documentation and detail of a member
// item that were left out of the completion list.
func (s *Server) handleCompletionResolve(msg *jsonRPCMessage) {
	var item protocol.CompletionItem
	if json.Unmarshal(msg.Params, &item) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	s.sendResponse(msg.ID, s.completion.ResolveCompletionItem(item))
}

func (s *Server) handleHover(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	uri := params.TextDocument.URI
	if cardhover.IsComposerJSON(uri) {
		s.sendResponse(msg.ID, s.composerHover.Hover(uri, s.getDocument(uri), params.Position))
		return
	}
	if h, ok := s.bladeHover(uri, params.Position); ok {
		s.sendResponse(msg.ID, h)
		return
	}
	req, ok := s.phpRequest(uri, params.Position)
	if !ok {
		s.sendResponse(msg.ID, nil)
		return
	}
	h := req.hover(s.hover.GetHover(req.uri, req.source, req.pos))
	if req.doc != nil {
		h = s.viewVarHover(h, req.vars, req.template, req.offset)
	}
	s.sendResponse(msg.ID, h)
}

func (s *Server) handleSignatureHelp(msg *jsonRPCMessage) {
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
	s.sendResponse(msg.ID, s.analyzer.GetSignatureHelp(req.uri, req.source, req.pos))
}

func (s *Server) handlePrepareRename(msg *jsonRPCMessage) {
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
	result := s.analyzer.PrepareRename(req.uri, req.source, req.pos)
	if result != nil && req.doc != nil {
		rng, ok := req.toView(result.Range)
		if !ok {
			s.sendResponse(msg.ID, nil)
			return
		}
		result.Range = rng
	}
	s.sendResponse(msg.ID, result)
}

func (s *Server) handleRename(msg *jsonRPCMessage) {
	var params protocol.RenameParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	req, ok := s.phpRequest(params.TextDocument.URI, params.Position)
	if !ok {
		s.sendResponse(msg.ID, nil)
		return
	}
	result := s.analyzer.Rename(req.uri, req.source, req.pos, params.NewName, s.getDocumentReader())
	s.sendResponse(msg.ID, req.workspaceEdit(result))
}

func (s *Server) handleCodeAction(msg *jsonRPCMessage) {
	var params protocol.CodeActionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	if workspace.IsBlade(params.TextDocument.URI) {
		s.sendResponse(msg.ID, analyzer.IgnoreDiagnosticActions(params.TextDocument.URI, source, params))
		return
	}
	result := s.analyzer.GetCodeActions(params.TextDocument.URI, source, params)
	if len(params.Context.Only) > 0 {
		filtered := result[:0]
		for _, action := range result {
			if action.Kind.In(params.Context.Only) {
				filtered = append(filtered, action)
			}
		}
		result = filtered
	}
	s.sendResponse(msg.ID, result)
}

func (s *Server) handleInlayHint(msg *jsonRPCMessage) {
	var params protocol.InlayHintParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	req := s.phpDocument(params.TextDocument.URI)
	hints := req.inlayHints(s.inlayHint.GetInlayHints(req.uri, req.source, &s.cfg.InlayHints))
	// Filter to the requested line range.
	filtered := hints[:0]
	for _, h := range hints {
		if h.Position.Line >= params.Range.Start.Line && h.Position.Line <= params.Range.End.Line {
			filtered = append(filtered, h)
		}
	}
	s.sendResponse(msg.ID, filtered)
}
