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

// findSymbolOccurrences scans all indexed files, and the PHP in templates, for
// references to the given symbol. Returns locations for both definition sites
// and usage sites.
//
// uri and source are the document the request came from. When it's a template,
// source is its virtual PHP and its locations are positions in that PHP, which
// the caller maps back; locations in other templates are mapped here.
func (a *Analyzer) findSymbolOccurrences(sym *symbols.Symbol, readDocument func(string) string, uri, source string) []protocol.Location {
	// Built-in symbols are opaque to navigation.
	if symbols.IsBuiltin(sym) {
		return nil
	}

	var locs []protocol.Location

	// Include the definition itself
	if sym.URI != "" && sym.AliasOf == nil {
		locs = append(locs, protocol.Location{URI: sym.URI, Range: sym.Range})
	}

	for _, fileURI := range a.index.GetAllFileURIs() {
		// Defense in depth: skip any stub URIs that may have been indexed.
		if strings.HasPrefix(fileURI, "builtin://") {
			continue
		}
		var src string
		if readDocument != nil {
			src = readDocument(fileURI)
		}
		// Prefer the in-memory source stored by the index to avoid redundant disk I/O.
		// Fall back to disk only when neither the caller-supplied reader nor the index
		// has a stored copy (e.g. files indexed without source via IndexFile on older paths).
		if src == "" {
			src = a.index.GetFileSource(fileURI)
		}
		if src == "" {
			src = a.readFileFromDisk(fileURI)
		}
		if src == "" {
			continue
		}
		locs = append(locs, symbolOccurrencesIn(sym, fileURI, src)...)
	}
	locs = append(locs, a.templateOccurrences(sym, readDocument, uri, source)...)

	// Deduplicate (definition may appear twice: once from sym.Range, once from file scan)
	return deduplicateLocations(locs)
}

// symbolOccurrencesIn finds the words in source that may name sym.
func symbolOccurrencesIn(sym *symbols.Symbol, fileURI, source string) []protocol.Location {
	var locs []protocol.Location
	name := sym.Name
	bareName := strings.TrimPrefix(name, "$")
	lineNum := 0
	for rest := source; ; lineNum++ {
		line := rest
		nl := strings.IndexByte(rest, '\n')
		if nl >= 0 {
			line = rest[:nl]
		}
		switch sym.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
			locs = append(locs, findWordLocations(fileURI, line, lineNum, name)...)
			// Also find as last segment of FQN (e.g. \Cacheable in use App\Contracts\Cacheable)
			locs = append(locs, findAccessLocations(fileURI, line, lineNum, "\\"+name, 1, name)...)

		case symbols.KindMethod:
			locs = append(locs, findWordLocations(fileURI, line, lineNum, name)...)

		case symbols.KindProperty:
			// Declaration form: $name
			locs = append(locs, findWordLocations(fileURI, line, lineNum, "$"+bareName)...)
			// Access form: ->name
			locs = append(locs, findAccessLocations(fileURI, line, lineNum, "->"+bareName, 2, bareName)...)

		case symbols.KindFunction:
			locs = append(locs, findWordLocations(fileURI, line, lineNum, name)...)

		case symbols.KindConstant, symbols.KindEnumCase:
			locs = append(locs, findWordLocations(fileURI, line, lineNum, name)...)
		}
		if nl < 0 {
			break
		}
		rest = rest[nl+1:]
	}
	return locs
}

// templateOccurrences finds sym in the PHP of the workspace's templates. Only
// PHP written in a template counts; what Blade generates around it never does.
func (a *Analyzer) templateOccurrences(sym *symbols.Symbol, readDocument func(string) string, uri, source string) []protocol.Location {
	templates := a.index.TemplateFileURIs()
	if workspace.IsBlade(uri) {
		// The requesting view is read from the request, in its PHP's positions.
		locs := symbolOccurrencesIn(sym, uri, source)
		templates = slices.DeleteFunc(templates, func(t string) bool { return t == uri })
		return append(locs, a.otherTemplateOccurrences(sym, readDocument, templates)...)
	}
	return a.otherTemplateOccurrences(sym, readDocument, templates)
}

func (a *Analyzer) otherTemplateOccurrences(sym *symbols.Symbol, readDocument func(string) string, templates []string) []protocol.Location {
	bareName := strings.TrimPrefix(sym.Name, "$")
	var locs []protocol.Location
	for _, t := range templates {
		var src string
		if readDocument != nil {
			src = readDocument(t)
		}
		if src == "" {
			src = a.readFileFromDisk(t)
		}
		// Most views don't mention the name at all: skip them unparsed.
		if src == "" || !strings.Contains(src, bareName) {
			continue
		}
		doc := blade.Compile(src)
		for _, l := range symbolOccurrencesIn(sym, t, doc.PHP) {
			r := l.Range
			sl, sc, el, ec, ok := doc.TemplateRange(r.Start.Line, r.Start.Character, r.End.Line, r.End.Character)
			// The whole name must be text the view contains.
			if !ok || sl != el || ec-sc != r.End.Character-r.Start.Character {
				continue
			}
			l.Range = protocol.Range{Start: protocol.Position{Line: sl, Character: sc}, End: protocol.Position{Line: el, Character: ec}}
			locs = append(locs, l)
		}
	}
	return locs
}

// findWordLocations finds all word-boundary occurrences of `word` on a line.
func findWordLocations(uri, line string, lineNum int, word string) []protocol.Location {
	var locs []protocol.Location
	offset := 0
	for {
		idx := strings.Index(line[offset:], word)
		if idx < 0 {
			break
		}
		col := offset + idx
		end := col + len(word)
		// Word boundary before
		if col > 0 && (resolve.IsWordChar(line[col-1]) || line[col-1] == '$') {
			offset = end
			continue
		}
		// Word boundary after
		if end < len(line) && resolve.IsWordChar(line[end]) {
			offset = end
			continue
		}
		locs = append(locs, protocol.Location{
			URI: uri,
			Range: protocol.Range{
				Start: protocol.Position{Line: lineNum, Character: col},
				End:   protocol.Position{Line: lineNum, Character: end},
			},
		})
		offset = end
	}
	return locs
}

// findAccessLocations finds occurrences of a needle (e.g. "->name") on a line
// and returns locations pointing to the identifier part (after skipChars).
func findAccessLocations(uri, line string, lineNum int, needle string, skipChars int, identName string) []protocol.Location {
	var locs []protocol.Location
	offset := 0
	for {
		idx := strings.Index(line[offset:], needle)
		if idx < 0 {
			break
		}
		col := offset + idx + skipChars
		end := col + len(identName)
		if end < len(line) && resolve.IsWordChar(line[end]) {
			offset = end
			continue
		}
		locs = append(locs, protocol.Location{
			URI: uri,
			Range: protocol.Range{
				Start: protocol.Position{Line: lineNum, Character: col},
				End:   protocol.Position{Line: lineNum, Character: end},
			},
		})
		offset = end
	}
	return locs
}
