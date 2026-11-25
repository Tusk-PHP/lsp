package lsp

import (
	"encoding/json"
	"os"
	"slices"

	"github.com/Tusk-PHP/lsp/internal/document"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type positions struct {
	enc     document.Encoding
	text    func(uri string) (string, bool)
	logf    func(format string, args ...any)
	columns map[string]*document.Columns
}

func (s *Server) positions() *positions {
	return &positions{enc: s.encoding, text: s.positionText, logf: s.logger.Printf}
}

func (s *Server) positionsOf(uri, source string) *positions {
	p := s.positions()
	p.columns = map[string]*document.Columns{uri: document.NewColumns(source, p.enc)}
	return p
}

func (s *Server) positionText(uri string) (string, bool) {
	s.docMu.RLock()
	source, open := s.documents[uri]
	s.docMu.RUnlock()
	if open {
		return source, true
	}
	if source := s.index.GetFileSource(uri); source != "" {
		return source, true
	}
	content, err := os.ReadFile(symbols.URIToPath(uri))
	if err != nil {
		return "", false
	}
	return string(content), true
}

func (p *positions) of(uri string) *document.Columns {
	if cols, ok := p.columns[uri]; ok {
		return cols
	}
	if p.columns == nil {
		p.columns = make(map[string]*document.Columns)
	}
	source, ok := p.text(uri)
	if !ok {
		p.logf("positions: no text for %s, its columns are sent as byte offsets", uri)
	}
	cols := document.NewColumns(source, p.enc)
	p.columns[uri] = cols
	return cols
}

func (p *positions) posIn(uri string, pos protocol.Position) protocol.Position {
	if p.identity(pos) {
		return pos
	}
	pos.Character = p.of(uri).FromClient(pos.Line, pos.Character)
	return pos
}

func (p *positions) rangeIn(uri string, r protocol.Range) protocol.Range {
	return protocol.Range{Start: p.posIn(uri, r.Start), End: p.posIn(uri, r.End)}
}

func (p *positions) diagnosticsIn(uri string, ds []protocol.Diagnostic) {
	for i := range ds {
		ds[i].Range = p.rangeIn(uri, ds[i].Range)
	}
}

func (p *positions) identity(pos protocol.Position) bool {
	return p.enc == document.UTF8 || pos.Character <= 0
}

func (p *positions) posOut(uri string, pos protocol.Position) protocol.Position {
	if p.identity(pos) {
		return pos
	}
	pos.Character = p.of(uri).ToClient(pos.Line, pos.Character)
	return pos
}

func (p *positions) rangeOut(uri string, r protocol.Range) protocol.Range {
	return protocol.Range{Start: p.posOut(uri, r.Start), End: p.posOut(uri, r.End)}
}

func (p *positions) rangePtrOut(uri string, r *protocol.Range) *protocol.Range {
	if r == nil {
		return nil
	}
	out := p.rangeOut(uri, *r)
	if out == *r {
		return r
	}
	return &out
}

func (p *positions) locationsOut(locs []protocol.Location) []protocol.Location {
	out, _ := convertEach(locs, func(l *protocol.Location) bool { return p.convertRange(l.URI, &l.Range) })
	return out
}

func (p *positions) locationOut(loc *protocol.Location) *protocol.Location {
	if loc == nil {
		return nil
	}
	out := protocol.Location{URI: loc.URI, Range: p.rangeOut(loc.URI, loc.Range)}
	return &out
}

func (p *positions) hoverOut(uri string, h *protocol.Hover) *protocol.Hover {
	if h != nil {
		h.Range = p.rangePtrOut(uri, h.Range)
	}
	return h
}

func (p *positions) textEditsOut(uri string, edits []protocol.TextEdit) []protocol.TextEdit {
	out, _ := p.convertTextEdits(uri, edits)
	return out
}

func (p *positions) convertTextEdits(uri string, edits []protocol.TextEdit) ([]protocol.TextEdit, bool) {
	return convertEach(edits, func(e *protocol.TextEdit) bool { return p.convertRange(uri, &e.Range) })
}

func (p *positions) completionsOut(uri string, items []protocol.CompletionItem) []protocol.CompletionItem {
	for i := range items {
		if e := items[i].TextEdit; e != nil {
			if r := p.rangeOut(uri, e.Range); r != e.Range {
				items[i].TextEdit = &protocol.TextEdit{Range: r, NewText: e.NewText}
			}
		}
		items[i].AdditionalTextEdits = p.textEditsOut(uri, items[i].AdditionalTextEdits)
	}
	return items
}

func (p *positions) workspaceEditOut(edit *protocol.WorkspaceEdit) *protocol.WorkspaceEdit {
	if edit == nil {
		return nil
	}
	out := &protocol.WorkspaceEdit{DocumentChanges: slices.Clone(edit.DocumentChanges)}
	if edit.Changes != nil {
		out.Changes = make(map[string][]protocol.TextEdit, len(edit.Changes))
		for uri, edits := range edit.Changes {
			out.Changes[uri] = p.textEditsOut(uri, edits)
		}
	}
	for i, change := range out.DocumentChanges {
		e := change.TextDocumentEdit
		if e == nil {
			continue
		}
		if edits, changed := p.convertTextEdits(e.TextDocument.URI, e.Edits); changed {
			out.DocumentChanges[i].TextDocumentEdit = &protocol.TextDocumentEdit{TextDocument: e.TextDocument, Edits: edits}
		}
	}
	return out
}

func (p *positions) diagnosticsOut(uri string, ds []protocol.Diagnostic) []protocol.Diagnostic {
	out, _ := convertEach(ds, func(d *protocol.Diagnostic) bool { return p.convertRange(uri, &d.Range) })
	return out
}

func (p *positions) codeActionsOut(uri string, actions []protocol.CodeAction) []protocol.CodeAction {
	for i := range actions {
		a := &actions[i]
		a.Diagnostics = p.diagnosticsOut(uri, a.Diagnostics)
		a.Edit = p.workspaceEditOut(a.Edit)
		if a.Command != nil && a.Command.Command == protocol.CommandCopyReference && len(a.Command.Arguments) == 2 {
			a.Command.Arguments[1] = p.copyReferencePositionOut(uri, a.Command.Arguments[1])
		}
	}
	return actions
}

func (p *positions) copyReferencePositionOut(uri string, arg json.RawMessage) json.RawMessage {
	var pos protocol.Position
	if err := json.Unmarshal(arg, &pos); err != nil {
		panic("copyReference argument is not a position: " + err.Error())
	}
	out, err := json.Marshal(p.posOut(uri, pos))
	if err != nil {
		panic("encoding a position: " + err.Error())
	}
	return out
}

func (p *positions) documentSymbolsOut(uri string, syms []protocol.DocumentSymbol) []protocol.DocumentSymbol {
	for i := range syms {
		syms[i].Range = p.rangeOut(uri, syms[i].Range)
		syms[i].SelectionRange = p.rangeOut(uri, syms[i].SelectionRange)
		p.documentSymbolsOut(uri, syms[i].Children)
	}
	return syms
}

func (p *positions) symbolInformationOut(syms []protocol.SymbolInformation) []protocol.SymbolInformation {
	for i := range syms {
		syms[i].Location.Range = p.rangeOut(syms[i].Location.URI, syms[i].Location.Range)
	}
	return syms
}

func (p *positions) highlightsOut(uri string, hs []protocol.DocumentHighlight) []protocol.DocumentHighlight {
	for i := range hs {
		hs[i].Range = p.rangeOut(uri, hs[i].Range)
	}
	return hs
}

func (p *positions) inlayHintsOut(uri string, hints []protocol.InlayHint) []protocol.InlayHint {
	for i := range hints {
		hints[i].Position = p.posOut(uri, hints[i].Position)
	}
	return hints
}

func (p *positions) selectionRangeOut(uri string, r protocol.SelectionRange) protocol.SelectionRange {
	for node := &r; node != nil; node = node.Parent {
		node.Range = p.rangeOut(uri, node.Range)
	}
	return r
}

func (p *positions) documentLinksOut(uri string, links []protocol.DocumentLink) []protocol.DocumentLink {
	for i := range links {
		links[i].Range = p.rangeOut(uri, links[i].Range)
	}
	return links
}

func (p *positions) typeHierarchyOut(items []protocol.TypeHierarchyItem) []protocol.TypeHierarchyItem {
	for i := range items {
		items[i].Range = p.rangeOut(items[i].URI, items[i].Range)
		items[i].SelectionRange = p.rangeOut(items[i].URI, items[i].SelectionRange)
	}
	return items
}

func (p *positions) prepareRenameOut(uri string, r *protocol.PrepareRenameResult) *protocol.PrepareRenameResult {
	if r != nil {
		r.Range = p.rangeOut(uri, r.Range)
	}
	return r
}

func (p *positions) convertRange(uri string, r *protocol.Range) bool {
	out := p.rangeOut(uri, *r)
	if out == *r {
		return false
	}
	*r = out
	return true
}

func convertEach[T any](items []T, convert func(*T) bool) ([]T, bool) {
	var out []T
	for i, item := range items {
		converted := item
		if !convert(&converted) {
			continue
		}
		if out == nil {
			out = slices.Clone(items)
		}
		out[i] = converted
	}
	if out == nil {
		return items, false
	}
	return out, true
}
