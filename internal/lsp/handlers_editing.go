package lsp

import (
	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/composer/cardhover"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

type completionParams struct {
	protocol.TextDocumentPositionParams
	Context *struct {
		TriggerCharacter string `json:"triggerCharacter"`
	} `json:"context"`
}

func (s *Server) handleCompletion(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[completionParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	p := s.positions()
	pos := p.posIn(uri, params.Position)
	if workspace.IsBlade(uri) {
		if items, ok := s.bladeCompletion(uri, pos); ok {
			s.sendResponse(msg.ID, p.completionsOut(uri, items))
			return
		}
	} else if params.Context != nil && params.Context.TriggerCharacter == "@" {
		// `@` starts a Blade directive; in PHP it has nothing to complete.
		s.sendResponse(msg.ID, []protocol.CompletionItem{})
		return
	}
	req, ok := s.phpRequest(uri, pos)
	if !ok {
		s.sendResponse(msg.ID, []protocol.CompletionItem{})
		return
	}
	items := req.completions(s.completion.GetCompletions(req.uri, req.source, req.pos))
	if req.doc != nil {
		items = withViewVarCompletions(items, req.vars, req.template, req.offset)
	}
	s.sendResponse(msg.ID, p.completionsOut(uri, items))
}

// handleCompletionResolve fills in the documentation and detail of a member
// item that were left out of the completion list.
func (s *Server) handleCompletionResolve(msg *jsonRPCMessage) {
	item, ok := s.decodeParams[protocol.CompletionItem](msg)
	if !ok {
		return
	}
	s.sendResponse(msg.ID, s.completion.ResolveCompletionItem(item))
}

func (s *Server) handleHover(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.TextDocumentPositionParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	p := s.positions()
	pos := p.posIn(uri, params.Position)
	if cardhover.IsComposerJSON(uri) {
		s.sendResponse(msg.ID, p.hoverOut(uri, s.composerHover.Hover(uri, s.getDocument(uri), pos)))
		return
	}
	if h, ok := s.bladeHover(uri, pos); ok {
		s.sendResponse(msg.ID, p.hoverOut(uri, h))
		return
	}
	req, ok := s.phpRequest(uri, pos)
	if !ok {
		s.sendResponse(msg.ID, nil)
		return
	}
	h := req.hover(s.hover.GetHover(req.uri, req.source, req.pos))
	if req.doc != nil {
		h = s.viewVarHover(h, req.vars, req.template, req.offset)
	}
	s.sendResponse(msg.ID, p.hoverOut(uri, h))
}

func (s *Server) handleSignatureHelp(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.TextDocumentPositionParams](msg)
	if !ok {
		return
	}
	req, ok := s.phpRequest(params.TextDocument.URI, s.positions().posIn(params.TextDocument.URI, params.Position))
	if !ok {
		s.sendResponse(msg.ID, nil)
		return
	}
	s.sendResponse(msg.ID, s.analyzer.GetSignatureHelp(req.uri, req.source, req.pos))
}

func (s *Server) handlePrepareRename(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.TextDocumentPositionParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	p := s.positions()
	req, ok := s.phpRequest(uri, p.posIn(uri, params.Position))
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
	s.sendResponse(msg.ID, p.prepareRenameOut(uri, result))
}

func (s *Server) handleRename(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.RenameParams](msg)
	if !ok {
		return
	}
	p := s.positions()
	req, ok := s.phpRequest(params.TextDocument.URI, p.posIn(params.TextDocument.URI, params.Position))
	if !ok {
		s.sendResponse(msg.ID, nil)
		return
	}
	result := s.analyzer.Rename(req.uri, req.source, req.pos, params.NewName, s.getDocumentReader())
	s.sendResponse(msg.ID, p.workspaceEditOut(req.workspaceEdit(result)))
}

func (s *Server) handleCodeAction(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.CodeActionParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	source := s.getDocument(uri)
	p := s.positionsOf(uri, source)
	params.Range = p.rangeIn(uri, params.Range)
	p.diagnosticsIn(uri, params.Context.Diagnostics)
	if workspace.IsBlade(uri) {
		s.sendResponse(msg.ID, p.codeActionsOut(uri, analyzer.BladeIgnoreDiagnosticActions(uri, source, params, s.providers.Custom())))
		return
	}
	result := s.analyzer.GetCodeActions(uri, source, params)
	if len(params.Context.Only) > 0 {
		filtered := result[:0]
		for _, action := range result {
			if action.Kind.In(params.Context.Only) {
				filtered = append(filtered, action)
			}
		}
		result = filtered
	}
	s.sendResponse(msg.ID, p.codeActionsOut(uri, result))
}

func (s *Server) handleInlayHint(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.InlayHintParams](msg)
	if !ok {
		return
	}
	uri := params.TextDocument.URI
	req := s.phpDocument(uri)
	hints := s.positionsOf(uri, s.getDocument(uri)).inlayHintsOut(uri, req.inlayHints(s.inlayHint.GetInlayHints(req.uri, req.source, &s.cfg.InlayHints)))
	// Filter to the requested line range.
	filtered := hints[:0]
	for _, h := range hints {
		if h.Position.Line >= params.Range.Start.Line && h.Position.Line <= params.Range.End.Line {
			filtered = append(filtered, h)
		}
	}
	s.sendResponse(msg.ID, filtered)
}
