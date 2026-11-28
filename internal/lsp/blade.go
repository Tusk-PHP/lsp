package lsp

import (
	"strings"
	"sync"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// Requests on a Blade view run on its virtual PHP: the position is moved into
// that PHP, the provider answers as it would for any PHP file, and positions in
// the view are moved back. Positions in generated PHP have no place in the view
// and are dropped. This is the only place request positions are mapped; the
// providers don't know about Blade.

// bladeViews keeps the compiled PHP of open Blade views, one entry per open
// document, dropped when the document closes. The zero value is ready to use.
type bladeViews struct {
	mu      sync.Mutex
	entries map[string]bladeEntry
}

type bladeEntry struct {
	source string
	vars   string // the declared variables, as a key
	custom *blade.Custom
	doc    *blade.Document
}

// get returns the compiled PHP of a view that receives vars, with the
// directives the project registers.
func (v *bladeViews) get(uri, source string, vars []blade.Var, custom *blade.Custom) *blade.Document {
	var key strings.Builder
	for _, bv := range vars {
		key.WriteString(bv.Name + ":" + bv.Type + ";")
	}
	v.mu.Lock()
	if e, ok := v.entries[uri]; ok && e.source == source && e.vars == key.String() && e.custom == custom {
		v.mu.Unlock()
		return e.doc
	}
	v.mu.Unlock()
	// Compile outside the lock; two requests racing on a new version both
	// compile it, which is harmless.
	doc := blade.CompileWith(source, blade.CompileOptions{Vars: vars, Custom: custom})
	v.mu.Lock()
	if v.entries == nil {
		v.entries = make(map[string]bladeEntry)
	}
	v.entries[uri] = bladeEntry{source: source, vars: key.String(), custom: custom, doc: doc}
	v.mu.Unlock()
	return doc
}

func (v *bladeViews) forget(uri string) {
	v.mu.Lock()
	delete(v.entries, uri)
	v.mu.Unlock()
}

// bladeRequest is a request on a document as the providers see it: for a Blade
// view its virtual PHP and the position in it, otherwise the document as is.
type bladeRequest struct {
	uri    string
	source string
	pos    protocol.Position
	doc    *blade.Document // nil unless the document is a Blade view
	// For a Blade view: its own text, the offset of the request in it, and
	// the variables passed to it.
	template string
	offset   int
	vars     []frameworklaravel.TypedVar
}

// phpRequest prepares a positional request. ok is false when the position is
// in a Blade view but outside the PHP in it, where PHP features have nothing
// to say.
func (s *Server) phpRequest(uri string, pos protocol.Position) (bladeRequest, bool) {
	source := s.getDocument(uri)
	if !workspace.IsBlade(uri) {
		return bladeRequest{uri: uri, source: source, pos: pos}, true
	}
	in := s.viewVars(uri)
	vars := in.Vars
	doc := s.blade.get(uri, source, in.BladeVars(), s.bladeCustom())
	line, col, ok := doc.PHPPosition(pos.Line, pos.Character)
	if !ok {
		return bladeRequest{}, false
	}
	return bladeRequest{
		uri: uri, source: doc.PHP, pos: protocol.Position{Line: line, Character: col}, doc: doc,
		template: source, offset: blade.NewLines(source).Offset(pos.Line, pos.Character), vars: vars,
	}, true
}

// phpDocument prepares a whole-document request, such as inlay hints.
func (s *Server) phpDocument(uri string) bladeRequest {
	source := s.getDocument(uri)
	if !workspace.IsBlade(uri) {
		return bladeRequest{uri: uri, source: source}
	}
	in := s.viewVars(uri)
	vars := in.Vars
	doc := s.blade.get(uri, source, in.BladeVars(), s.bladeCustom())
	return bladeRequest{uri: uri, source: doc.PHP, doc: doc, template: source, vars: vars}
}

// text is the document the request was made on, as the client sent it.
func (r bladeRequest) text() string {
	if r.doc != nil {
		return r.template
	}
	return r.source
}

// toView moves a range in the virtual PHP to the view.
func (r bladeRequest) toView(rng protocol.Range) (protocol.Range, bool) {
	if r.doc == nil {
		return rng, true
	}
	sl, sc, el, ec, ok := r.doc.TemplateRange(rng.Start.Line, rng.Start.Character, rng.End.Line, rng.End.Character)
	if !ok {
		return protocol.Range{}, false
	}
	return protocol.Range{Start: protocol.Position{Line: sl, Character: sc}, End: protocol.Position{Line: el, Character: ec}}, true
}

// locations moves the locations in this document to the view and drops those
// in generated PHP. Locations in other files are left alone.
func (r bladeRequest) locations(locs []protocol.Location) []protocol.Location {
	if r.doc == nil {
		return locs
	}
	out := make([]protocol.Location, 0, len(locs))
	for _, l := range locs {
		if l.URI == r.uri {
			rng, ok := r.toView(l.Range)
			if !ok {
				continue
			}
			l.Range = rng
		}
		out = append(out, l)
	}
	return out
}

func (r bladeRequest) location(loc *protocol.Location) *protocol.Location {
	if r.doc == nil || loc == nil {
		return loc
	}
	locs := r.locations([]protocol.Location{*loc})
	if len(locs) == 0 {
		return nil
	}
	return &locs[0]
}

func (r bladeRequest) hover(h *protocol.Hover) *protocol.Hover {
	if r.doc == nil || h == nil || h.Range == nil {
		return h
	}
	if rng, ok := r.toView(*h.Range); ok {
		h.Range = &rng
	} else {
		h.Range = nil
	}
	return h
}

// completions moves the items' extra edits to the view. An edit that falls in
// generated PHP is an import added at the top of the file, which a view has no
// place for: the item then inserts the fully qualified name instead.
func (r bladeRequest) completions(items []protocol.CompletionItem) []protocol.CompletionItem {
	if r.doc == nil {
		return items
	}
	for i := range items {
		item := &items[i]
		if len(item.AdditionalTextEdits) == 0 {
			continue
		}
		edits := make([]protocol.TextEdit, 0, len(item.AdditionalTextEdits))
		imported := false
		for _, e := range item.AdditionalTextEdits {
			rng, ok := r.toView(e.Range)
			if !ok {
				imported = true
				continue
			}
			e.Range = rng
			edits = append(edits, e)
		}
		if imported {
			edits = nil
			if fqn := item.Detail; isClassName(fqn) {
				insert := item.InsertText
				if insert == "" {
					insert = item.Label
				}
				// Keep what follows the name, such as `($0)` after `new Name`.
				rest := ""
				if k := strings.IndexByte(insert, '('); k >= 0 {
					rest = insert[k:]
				}
				item.InsertText = "\\" + strings.TrimPrefix(fqn, "\\") + rest
			}
		}
		item.AdditionalTextEdits = edits
	}
	return items
}

// isClassName reports whether s looks like a qualified class name.
func isClassName(s string) bool {
	if s == "" || strings.ContainsAny(s, " ()$:<>|&?") {
		return false
	}
	return true
}

func (r bladeRequest) highlights(hs []protocol.DocumentHighlight) []protocol.DocumentHighlight {
	if r.doc == nil {
		return hs
	}
	out := make([]protocol.DocumentHighlight, 0, len(hs))
	for _, h := range hs {
		if rng, ok := r.toView(h.Range); ok {
			h.Range = rng
			out = append(out, h)
		}
	}
	return out
}

func (r bladeRequest) inlayHints(hints []protocol.InlayHint) []protocol.InlayHint {
	if r.doc == nil {
		return hints
	}
	out := make([]protocol.InlayHint, 0, len(hints))
	for _, h := range hints {
		line, col, ok := r.doc.TemplatePosition(h.Position.Line, h.Position.Character)
		if !ok {
			continue
		}
		h.Position = protocol.Position{Line: line, Character: col}
		out = append(out, h)
	}
	return out
}

func (r bladeRequest) workspaceEdit(edit *protocol.WorkspaceEdit) *protocol.WorkspaceEdit {
	if r.doc == nil || edit == nil {
		return edit
	}
	edits, ok := edit.Changes[r.uri]
	if !ok {
		return edit
	}
	out := make([]protocol.TextEdit, 0, len(edits))
	for _, e := range edits {
		if rng, ok := r.toView(e.Range); ok {
			e.Range = rng
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		delete(edit.Changes, r.uri)
	} else {
		edit.Changes[r.uri] = out
	}
	return edit
}
