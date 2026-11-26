package lsp

import (
	"path/filepath"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// Requests about Blade itself (directives, and the views, sections and stacks
// they name) are answered by blade.Features on the template's own text. Each
// returns ok false when the position is about PHP instead, which is then
// answered through the view's virtual PHP (see blade.go).

func (s *Server) bladeCompletion(uri string, pos protocol.Position) ([]protocol.CompletionItem, bool) {
	if s.session == nil {
		return nil, false
	}
	src := s.getDocument(uri)
	lines := blade.NewLines(src)
	offset := lines.Offset(pos.Line, pos.Character)
	if offset < 0 || offset > len(src) {
		return nil, false
	}
	found, ok := s.session.Blade().Complete(uri, src, offset)
	if !ok {
		return nil, false
	}
	items := make([]protocol.CompletionItem, 0, len(found))
	for i, c := range found {
		item := protocol.CompletionItem{
			Label:         c.Label,
			Detail:        c.Detail,
			Documentation: c.Doc,
			FilterText:    c.Label,
			SortText:      sortKey(i),
			TextEdit:      &protocol.TextEdit{Range: offsetRange(lines, c.Start, c.End), NewText: c.Insert},
		}
		switch c.Kind {
		case blade.CompleteDirective:
			item.Kind = protocol.CompletionItemKindKeyword
		case blade.CompleteView:
			item.Kind = protocol.CompletionItemKindFile
		case blade.CompleteComponent:
			item.Kind = protocol.CompletionItemKindClass
		case blade.CompleteAttribute:
			item.Kind = protocol.CompletionItemKindProperty
		default:
			item.Kind = protocol.CompletionItemKindReference
		}
		if c.Snippet {
			item.InsertTextFormat = 2
		}
		items = append(items, item)
	}
	return items, true
}

func (s *Server) bladeHover(uri string, pos protocol.Position) (*protocol.Hover, bool) {
	if s.session == nil || !workspace.IsBlade(uri) {
		return nil, false
	}
	src := s.getDocument(uri)
	lines := blade.NewLines(src)
	offset := lines.Offset(pos.Line, pos.Character)
	if offset < 0 || offset > len(src) {
		return nil, false
	}
	h, ok := s.session.Blade().Hover(src, offset)
	if !ok {
		return nil, false
	}
	rng := offsetRange(lines, h.Span.Start, h.Span.End)
	return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: h.Markdown}, Range: &rng}, true
}

func (s *Server) bladeDefinition(uri string, pos protocol.Position) ([]protocol.Location, bool) {
	if s.session == nil || !workspace.IsBlade(uri) {
		return nil, false
	}
	src := s.getDocument(uri)
	lines := blade.NewLines(src)
	offset := lines.Offset(pos.Line, pos.Character)
	if offset < 0 || offset > len(src) {
		return nil, false
	}
	found, ok := s.session.Blade().Definition(uri, src, offset)
	if !ok {
		return nil, false
	}
	locs := make([]protocol.Location, 0, len(found))
	for _, l := range found {
		locs = append(locs, protocol.Location{URI: l.URI, Range: protocol.Range{
			Start: protocol.Position{Line: l.Line, Character: l.Col},
			End:   protocol.Position{Line: l.Line, Character: l.EndCol},
		}})
	}
	return locs, true
}

func offsetRange(lines blade.Lines, start, end int) protocol.Range {
	sl, sc := lines.Position(start)
	el, ec := lines.Position(end)
	return protocol.Range{Start: protocol.Position{Line: sl, Character: sc}, End: protocol.Position{Line: el, Character: ec}}
}

// sortKey keeps items in the order given.
func sortKey(i int) string {
	const digits = "0123456789"
	return string([]byte{digits[i/1000%10], digits[i/100%10], digits[i/10%10], digits[i%10]})
}

// affectsComponents reports whether a saved file can change which Blade
// components exist: a provider, a component class or an anonymous component.
func affectsComponents(root, uri string) bool {
	path, err := symbols.URIToPath(uri)
	if err != nil {
		return false
	}
	path = filepath.ToSlash(path)
	root = strings.TrimSuffix(filepath.ToSlash(root), "/") + "/"
	if !strings.HasPrefix(path, root) {
		return false
	}
	rel := path[len(root):]
	return strings.HasPrefix(rel, "app/Providers/") || strings.HasPrefix(rel, "app/View/Components/") ||
		strings.HasPrefix(rel, "resources/views/components/")
}
