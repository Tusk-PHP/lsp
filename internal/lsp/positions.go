package lsp

import (
	"encoding/json"
	"maps"
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

func (p *positions) diagnosticsIn(uri string, ds []protocol.Diagnostic) []protocol.Diagnostic {
	out, _ := convertEach(ds, func(d *protocol.Diagnostic) bool { return convertTo(&d.Range, p.rangeIn(uri, d.Range)) })
	return out
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
	if h == nil {
		return nil
	}
	r := p.rangePtrOut(uri, h.Range)
	if r == h.Range {
		return h
	}
	out := *h
	out.Range = r
	return &out
}

func (p *positions) convertTextEdits(uri string, edits []protocol.TextEdit) ([]protocol.TextEdit, bool) {
	return convertEach(edits, func(e *protocol.TextEdit) bool { return p.convertRange(uri, &e.Range) })
}

func (p *positions) completionsOut(uri string, items []protocol.CompletionItem) []protocol.CompletionItem {
	out, _ := convertEach(items, func(item *protocol.CompletionItem) bool {
		changed := false
		if e := item.TextEdit; e != nil {
			if r := p.rangeOut(uri, e.Range); r != e.Range {
				item.TextEdit = &protocol.TextEdit{Range: r, NewText: e.NewText}
				changed = true
			}
		}
		if edits, ok := p.convertTextEdits(uri, item.AdditionalTextEdits); ok {
			item.AdditionalTextEdits = edits
			changed = true
		}
		return changed
	})
	return out
}

func (p *positions) workspaceEditOut(edit *protocol.WorkspaceEdit) *protocol.WorkspaceEdit {
	out, _ := p.convertWorkspaceEdit(edit)
	return out
}

func (p *positions) convertWorkspaceEdit(edit *protocol.WorkspaceEdit) (*protocol.WorkspaceEdit, bool) {
	if edit == nil {
		return nil, false
	}
	changes, changesConverted := p.convertChanges(edit.Changes)
	documentChanges, documentChangesConverted := convertEach(edit.DocumentChanges, func(change *protocol.DocumentChange) bool {
		e := change.TextDocumentEdit
		if e == nil {
			return false
		}
		edits, converted := p.convertTextEdits(e.TextDocument.URI, e.Edits)
		if converted {
			change.TextDocumentEdit = &protocol.TextDocumentEdit{TextDocument: e.TextDocument, Edits: edits}
		}
		return converted
	})
	if !changesConverted && !documentChangesConverted {
		return edit, false
	}
	out := *edit
	out.Changes = changes
	out.DocumentChanges = documentChanges
	return &out, true
}

func (p *positions) convertChanges(changes map[string][]protocol.TextEdit) (map[string][]protocol.TextEdit, bool) {
	var out map[string][]protocol.TextEdit
	for uri, edits := range changes {
		converted, ok := p.convertTextEdits(uri, edits)
		if !ok {
			continue
		}
		if out == nil {
			out = maps.Clone(changes)
		}
		out[uri] = converted
	}
	if out == nil {
		return changes, false
	}
	return out, true
}

func (p *positions) diagnosticsOut(uri string, ds []protocol.Diagnostic) []protocol.Diagnostic {
	out, _ := p.convertDiagnostics(uri, ds)
	return out
}

func (p *positions) convertDiagnostics(uri string, ds []protocol.Diagnostic) ([]protocol.Diagnostic, bool) {
	return convertEach(ds, func(d *protocol.Diagnostic) bool { return p.convertRange(uri, &d.Range) })
}

func (p *positions) codeActionsOut(uri string, actions []protocol.CodeAction) []protocol.CodeAction {
	out, _ := convertEach(actions, func(a *protocol.CodeAction) bool {
		diagnostics, diagnosticsConverted := p.convertDiagnostics(uri, a.Diagnostics)
		edit, editConverted := p.convertWorkspaceEdit(a.Edit)
		command, commandConverted := p.convertCommand(uri, a.Command)
		a.Diagnostics, a.Edit, a.Command = diagnostics, edit, command
		return diagnosticsConverted || editConverted || commandConverted
	})
	return out
}

func (p *positions) convertCommand(uri string, c *protocol.Command) (*protocol.Command, bool) {
	if c == nil || c.Command != protocol.CommandCopyReference || len(c.Arguments) != 2 {
		return c, false
	}
	var pos protocol.Position
	if err := json.Unmarshal(c.Arguments[1], &pos); err != nil {
		panic("copyReference argument is not a position: " + err.Error())
	}
	converted := p.posOut(uri, pos)
	if converted == pos {
		return c, false
	}
	arg, err := json.Marshal(converted)
	if err != nil {
		panic("encoding a position: " + err.Error())
	}
	out := *c
	out.Arguments = []json.RawMessage{c.Arguments[0], arg}
	return &out, true
}

func (p *positions) documentSymbolsOut(uri string, syms []protocol.DocumentSymbol) []protocol.DocumentSymbol {
	out, _ := p.convertDocumentSymbols(uri, syms)
	return out
}

func (p *positions) convertDocumentSymbols(uri string, syms []protocol.DocumentSymbol) ([]protocol.DocumentSymbol, bool) {
	return convertEach(syms, func(sym *protocol.DocumentSymbol) bool {
		rangeConverted := p.convertRange(uri, &sym.Range)
		selectionConverted := p.convertRange(uri, &sym.SelectionRange)
		children, childrenConverted := p.convertDocumentSymbols(uri, sym.Children)
		sym.Children = children
		return rangeConverted || selectionConverted || childrenConverted
	})
}

func (p *positions) symbolInformationOut(syms []protocol.SymbolInformation) []protocol.SymbolInformation {
	out, _ := convertEach(syms, func(sym *protocol.SymbolInformation) bool {
		return p.convertRange(sym.Location.URI, &sym.Location.Range)
	})
	return out
}

func (p *positions) highlightsOut(uri string, hs []protocol.DocumentHighlight) []protocol.DocumentHighlight {
	out, _ := convertEach(hs, func(h *protocol.DocumentHighlight) bool { return p.convertRange(uri, &h.Range) })
	return out
}

func (p *positions) inlayHintsOut(uri string, hints []protocol.InlayHint) []protocol.InlayHint {
	out, _ := convertEach(hints, func(h *protocol.InlayHint) bool { return convertTo(&h.Position, p.posOut(uri, h.Position)) })
	return out
}

func (p *positions) selectionRangeOut(uri string, r protocol.SelectionRange) protocol.SelectionRange {
	if r.Parent != nil {
		if parent := p.selectionRangeOut(uri, *r.Parent); parent != *r.Parent {
			r.Parent = &parent
		}
	}
	r.Range = p.rangeOut(uri, r.Range)
	return r
}

func (p *positions) documentLinksOut(uri string, links []protocol.DocumentLink) []protocol.DocumentLink {
	out, _ := convertEach(links, func(l *protocol.DocumentLink) bool { return p.convertRange(uri, &l.Range) })
	return out
}

func (p *positions) typeHierarchyOut(items []protocol.TypeHierarchyItem) []protocol.TypeHierarchyItem {
	out, _ := convertEach(items, func(item *protocol.TypeHierarchyItem) bool {
		rangeConverted := p.convertRange(item.URI, &item.Range)
		selectionConverted := p.convertRange(item.URI, &item.SelectionRange)
		return rangeConverted || selectionConverted
	})
	return out
}

func (p *positions) prepareRenameOut(uri string, r *protocol.PrepareRenameResult) *protocol.PrepareRenameResult {
	if r == nil {
		return nil
	}
	out := *r
	if !p.convertRange(uri, &out.Range) {
		return r
	}
	return &out
}

func (p *positions) convertRange(uri string, r *protocol.Range) bool {
	return convertTo(r, p.rangeOut(uri, *r))
}

func convertTo[T comparable](target *T, converted T) bool {
	if converted == *target {
		return false
	}
	*target = converted
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
