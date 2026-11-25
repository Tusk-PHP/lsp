package analyzer

import (
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// findSymbolOccurrences finds the references to sym, and to the members of
// its family (symbols.MemberFamily), in every indexed file and in the PHP of
// templates, their declarations included.
//
// uri and source are the document the request came from. When it's a template,
// source is its virtual PHP and its locations are positions in that PHP, which
// the caller maps back; locations in other templates are mapped here.
func (a *Analyzer) findSymbolOccurrences(r *resolve.Resolver, sym *symbols.Symbol, readDocument func(string) string, uri, source string) []protocol.Location {
	if symbols.IsBuiltin(sym) {
		return nil
	}
	target := a.referenceTargetOf(sym)
	locs := target.declarations()
	uris := slices.DeleteFunc(a.index.GetAllFileURIs(), func(u string) bool { return strings.HasPrefix(u, "builtin://") })
	for i, src := range a.mentioningSources(uris, sym, readDocument) {
		if src != "" {
			locs = append(locs, locationsIn(uris[i], a.occurrencesInDocument(r, target, sourcectx.NewDocument(uris[i], src), src))...)
		}
	}
	locs = append(locs, a.templateOccurrences(r, target, readDocument, uri, source)...)
	return deduplicateLocations(locs)
}

// mentioningSources reads each file of uris and keeps its source when a token
// in it may name sym, "" otherwise. Reading and lexing are independent per
// file, so they run on one worker per CPU.
func (a *Analyzer) mentioningSources(uris []string, sym *symbols.Symbol, readDocument func(string) string) []string {
	sources := make([]string, len(uris))
	next := atomic.Int64{}
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), len(uris)) {
		wg.Go(func() {
			for i := int(next.Add(1) - 1); i < len(uris); i = int(next.Add(1) - 1) {
				if src := a.sourceOf(uris[i], readDocument); mayMention(src, sym) && mentions(src, sym) {
					sources[i] = src
				}
			}
		})
	}
	wg.Wait()
	return sources
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

func (a *Analyzer) templateOccurrences(r *resolve.Resolver, target referenceTarget, readDocument func(string) string, uri, source string) []protocol.Location {
	templates := a.index.TemplateFileURIs()
	if workspace.IsBlade(uri) {
		locs := locationsIn(uri, a.occurrencesIn(r, target, uri, source))
		templates = slices.DeleteFunc(templates, func(t string) bool { return t == uri })
		return append(locs, a.otherTemplateOccurrences(r, target, readDocument, templates)...)
	}
	return a.otherTemplateOccurrences(r, target, readDocument, templates)
}

func (a *Analyzer) otherTemplateOccurrences(r *resolve.Resolver, target referenceTarget, readDocument func(string) string, templates []string) []protocol.Location {
	var locs []protocol.Location
	for _, t := range templates {
		var src string
		if readDocument != nil {
			src = readDocument(t)
		}
		if src == "" {
			src = a.readFileFromDisk(t)
		}
		if !mayMention(src, target.sym) {
			continue
		}
		doc := blade.Compile(src)
		for _, rng := range a.occurrencesIn(r, target, t, doc.PHP) {
			sl, sc, el, ec, ok := doc.TemplateRange(rng.Start.Line, rng.Start.Character, rng.End.Line, rng.End.Character)
			if !ok || sl != el || ec-sc != rng.End.Character-rng.Start.Character {
				continue
			}
			locs = append(locs, protocol.Location{URI: t, Range: protocol.Range{Start: protocol.Position{Line: sl, Character: sc}, End: protocol.Position{Line: el, Character: ec}}})
		}
	}
	return locs
}
