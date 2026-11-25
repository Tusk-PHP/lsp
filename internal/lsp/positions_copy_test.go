package lsp

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/document"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

const copyURI = "file:///copy.php"

func copyPositions() *positions {
	return &positions{
		enc:  document.UTF16,
		text: func(string) (string, bool) { return "<?php $é = 1; $x = 2;\n", true },
		logf: func(string, ...any) {},
	}
}

func copyRange() protocol.Range {
	return protocol.Range{Start: protocol.Position{Line: 0, Character: 15}, End: protocol.Position{Line: 0, Character: 17}}
}

func copyEdits() []protocol.TextEdit {
	return []protocol.TextEdit{{Range: copyRange(), NewText: "y"}}
}

func copyWorkspaceEdit() *protocol.WorkspaceEdit {
	return &protocol.WorkspaceEdit{
		Changes: map[string][]protocol.TextEdit{copyURI: copyEdits()},
		DocumentChanges: []protocol.DocumentChange{{TextDocumentEdit: &protocol.TextDocumentEdit{
			TextDocument: protocol.VersionedTextDocumentIdentifier{URI: copyURI},
			Edits:        copyEdits(),
		}}},
	}
}

func TestPositionConversionsLeaveTheProviderResultUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		make    func() any
		convert func(p *positions, v any) any
	}{
		{"diagnostics in", func() any { return []protocol.Diagnostic{{Range: copyRange()}} },
			func(p *positions, v any) any { return p.diagnosticsIn(copyURI, v.([]protocol.Diagnostic)) }},
		{"diagnostics", func() any { return []protocol.Diagnostic{{Range: copyRange()}} },
			func(p *positions, v any) any { return p.diagnosticsOut(copyURI, v.([]protocol.Diagnostic)) }},
		{"hover", func() any { r := copyRange(); return &protocol.Hover{Range: &r} },
			func(p *positions, v any) any { return p.hoverOut(copyURI, v.(*protocol.Hover)) }},
		{"completions", func() any {
			return []protocol.CompletionItem{{Label: "x", TextEdit: &protocol.TextEdit{Range: copyRange()}, AdditionalTextEdits: copyEdits()}}
		}, func(p *positions, v any) any { return p.completionsOut(copyURI, v.([]protocol.CompletionItem)) }},
		{"workspace edit", func() any { return copyWorkspaceEdit() },
			func(p *positions, v any) any { return p.workspaceEditOut(v.(*protocol.WorkspaceEdit)) }},
		{"code actions", func() any {
			arg, _ := json.Marshal(copyRange().Start)
			return []protocol.CodeAction{{
				Diagnostics: []protocol.Diagnostic{{Range: copyRange()}},
				Edit:        copyWorkspaceEdit(),
				Command:     &protocol.Command{Command: protocol.CommandCopyReference, Arguments: []json.RawMessage{json.RawMessage(`"App\\Foo"`), arg}},
			}}
		}, func(p *positions, v any) any { return p.codeActionsOut(copyURI, v.([]protocol.CodeAction)) }},
		{"document symbols", func() any {
			return []protocol.DocumentSymbol{{Range: copyRange(), SelectionRange: copyRange(), Children: []protocol.DocumentSymbol{{Range: copyRange()}}}}
		}, func(p *positions, v any) any { return p.documentSymbolsOut(copyURI, v.([]protocol.DocumentSymbol)) }},
		{"workspace symbols", func() any {
			return []protocol.SymbolInformation{{Location: protocol.Location{URI: copyURI, Range: copyRange()}}}
		}, func(p *positions, v any) any { return p.symbolInformationOut(v.([]protocol.SymbolInformation)) }},
		{"highlights", func() any { return []protocol.DocumentHighlight{{Range: copyRange()}} },
			func(p *positions, v any) any { return p.highlightsOut(copyURI, v.([]protocol.DocumentHighlight)) }},
		{"inlay hints", func() any { return []protocol.InlayHint{{Position: copyRange().Start}} },
			func(p *positions, v any) any { return p.inlayHintsOut(copyURI, v.([]protocol.InlayHint)) }},
		{"selection range", func() any {
			return &protocol.SelectionRange{Range: copyRange(), Parent: &protocol.SelectionRange{Range: copyRange(), Parent: &protocol.SelectionRange{Range: copyRange()}}}
		}, func(p *positions, v any) any {
			out := p.selectionRangeOut(copyURI, *v.(*protocol.SelectionRange))
			return &out
		}},
		{"document links", func() any { return []protocol.DocumentLink{{Range: copyRange()}} },
			func(p *positions, v any) any { return p.documentLinksOut(copyURI, v.([]protocol.DocumentLink)) }},
		{"type hierarchy", func() any {
			return []protocol.TypeHierarchyItem{{URI: copyURI, Range: copyRange(), SelectionRange: copyRange()}}
		}, func(p *positions, v any) any { return p.typeHierarchyOut(v.([]protocol.TypeHierarchyItem)) }},
		{"prepare rename", func() any { return &protocol.PrepareRenameResult{Range: copyRange()} },
			func(p *positions, v any) any { return p.prepareRenameOut(copyURI, v.(*protocol.PrepareRenameResult)) }},
		{"locations", func() any { return []protocol.Location{{URI: copyURI, Range: copyRange()}} },
			func(p *positions, v any) any { return p.locationsOut(v.([]protocol.Location)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.make()
			got := tc.convert(copyPositions(), input)
			if !reflect.DeepEqual(input, tc.make()) {
				t.Errorf("the conversion changed its input: %+v", input)
			}
			if reflect.DeepEqual(got, tc.make()) {
				t.Errorf("columns after é were not converted: %+v", got)
			}
			if again := tc.convert(copyPositions(), input); !reflect.DeepEqual(again, got) {
				t.Errorf("converting the same input twice gave %+v, then %+v", got, again)
			}
		})
	}
}
