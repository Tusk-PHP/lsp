package analyzer

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// findSymbolOccurrences finds the references to sym in every indexed file
// and in the PHP of templates, its declaration included.
//
// uri and source are the document the request came from. When it's a template,
// source is its virtual PHP and its locations are positions in that PHP, which
// the caller maps back; locations in other templates are mapped here.
func (a *Analyzer) findSymbolOccurrences(sym *symbols.Symbol, readDocument func(string) string, uri, source string) []protocol.Location {
	if symbols.IsBuiltin(sym) {
		return nil
	}
	var locs []protocol.Location
	if sym.URI != "" {
		locs = append(locs, protocol.Location{URI: sym.URI, Range: sym.Range})
	}
	for _, fileURI := range a.index.GetAllFileURIs() {
		if strings.HasPrefix(fileURI, "builtin://") {
			continue
		}
		src := a.sourceOf(fileURI, readDocument)
		if !mayMention(src, sym) {
			continue
		}
		locs = append(locs, locationsIn(fileURI, a.occurrencesIn(sym, fileURI, src))...)
	}
	locs = append(locs, a.templateOccurrences(sym, readDocument, uri, source)...)
	return deduplicateLocations(locs)
}

func (a *Analyzer) sourceOf(uri string, readDocument func(string) string) string {
	if readDocument != nil {
		if src := readDocument(uri); src != "" {
			return src
		}
	}
	if src := a.index.GetFileSource(uri); src != "" {
		return src
	}
	return a.readFileFromDisk(uri)
}

func mayMention(src string, sym *symbols.Symbol) bool {
	name := strings.TrimPrefix(sym.Name, "$")
	for offset := 0; ; {
		i := strings.Index(src[offset:], name)
		if i < 0 {
			return false
		}
		start, end := offset+i, offset+i+len(name)
		if (start == 0 || !resolve.IsWordChar(src[start-1]) && (src[start-1] != '$' || sym.Kind == symbols.KindProperty)) &&
			(end == len(src) || !resolve.IsWordChar(src[end])) {
			return true
		}
		offset = end
	}
}

func locationsIn(uri string, ranges []protocol.Range) []protocol.Location {
	locs := make([]protocol.Location, len(ranges))
	for i, r := range ranges {
		locs[i] = protocol.Location{URI: uri, Range: r}
	}
	return locs
}

func (a *Analyzer) templateOccurrences(sym *symbols.Symbol, readDocument func(string) string, uri, source string) []protocol.Location {
	templates := a.index.TemplateFileURIs()
	if workspace.IsBlade(uri) {
		locs := locationsIn(uri, a.occurrencesIn(sym, uri, source))
		templates = slices.DeleteFunc(templates, func(t string) bool { return t == uri })
		return append(locs, a.otherTemplateOccurrences(sym, readDocument, templates)...)
	}
	return a.otherTemplateOccurrences(sym, readDocument, templates)
}

func (a *Analyzer) otherTemplateOccurrences(sym *symbols.Symbol, readDocument func(string) string, templates []string) []protocol.Location {
	var locs []protocol.Location
	for _, t := range templates {
		var src string
		if readDocument != nil {
			src = readDocument(t)
		}
		if src == "" {
			src = a.readFileFromDisk(t)
		}
		if !mayMention(src, sym) {
			continue
		}
		doc := blade.Compile(src)
		for _, r := range a.occurrencesIn(sym, t, doc.PHP) {
			sl, sc, el, ec, ok := doc.TemplateRange(r.Start.Line, r.Start.Character, r.End.Line, r.End.Character)
			if !ok || sl != el || ec-sc != r.End.Character-r.Start.Character {
				continue
			}
			locs = append(locs, protocol.Location{URI: t, Range: protocol.Range{Start: protocol.Position{Line: sl, Character: sc}, End: protocol.Position{Line: el, Character: ec}}})
		}
	}
	return locs
}
