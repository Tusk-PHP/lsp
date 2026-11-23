package lsp

import "fmt"

func (s *Server) handleMessage(msg *jsonRPCMessage) {
	// Responses to server-initiated requests (e.g. window/showDocument) are
	// fire-and-forget; we drop them silently.
	if msg.Method == "" && msg.ID != nil {
		return
	}
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		s.handleInitialized(msg)
	case "shutdown":
		s.shutdown = true
		s.cancelAllDiagnostics()
		if s.coverage != nil {
			s.coverage.Stop()
		}
		s.sendResponse(msg.ID, nil)
	case "exit":
		s.exitFunc(0)
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/didSave":
		s.handleDidSave(msg)
	case "textDocument/completion":
		s.handleCompletion(msg)
	case "completionItem/resolve":
		s.handleCompletionResolve(msg)
	case "textDocument/hover":
		s.handleHover(msg)
	case "textDocument/definition":
		s.handleDefinition(msg)
	case "textDocument/prepareTypeHierarchy":
		s.handlePrepareTypeHierarchy(msg)
	case "typeHierarchy/supertypes":
		s.handleTypeHierarchyRelations(msg, s.analyzer.Supertypes)
	case "typeHierarchy/subtypes":
		s.handleTypeHierarchyRelations(msg, s.analyzer.Subtypes)
	case "textDocument/typeDefinition":
		s.handleTypeDefinition(msg)
	case "textDocument/implementation":
		s.handleImplementation(msg)
	case "textDocument/references":
		s.handleReferences(msg)
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(msg)
	case "textDocument/foldingRange":
		s.handleFoldingRange(msg)
	case "textDocument/documentHighlight":
		s.handleDocumentHighlight(msg)
	case "textDocument/signatureHelp":
		s.handleSignatureHelp(msg)
	case "textDocument/prepareRename":
		s.handlePrepareRename(msg)
	case "textDocument/rename":
		s.handleRename(msg)
	case "textDocument/codeAction":
		s.handleCodeAction(msg)
	case "textDocument/inlayHint":
		s.handleInlayHint(msg)
	case "textDocument/selectionRange":
		s.handleSelectionRange(msg)
	case "textDocument/semanticTokens/full":
		s.handleSemanticTokensFull(msg)
	case "workspace/symbol":
		s.handleWorkspaceSymbol(msg)
	case "workspace/executeCommand":
		s.handleExecuteCommand(msg)
	case "tusk/coverageReport":
		s.handleCoverageReport(msg)
	case "tusk/fileCoverage":
		s.handleFileCoverage(msg)
	default:
		if msg.ID != nil {
			s.sendError(msg.ID, -32601, fmt.Sprintf("Method not found: %s", msg.Method))
		}
	}
}
