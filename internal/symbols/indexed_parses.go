package symbols

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
)

const keptParses = 4

type indexedParse struct {
	uri    string
	text   string
	parsed phpparser.Parsed
	file   *parser.FileNode
}

type indexedParses struct {
	entries [keptParses]indexedParse
	next    int
}

func (p *indexedParses) put(entry indexedParse) {
	for i := range p.entries {
		if p.entries[i].uri == entry.uri {
			p.entries[i] = entry
			return
		}
	}
	p.entries[p.next] = entry
	p.next = (p.next + 1) % keptParses
}

func (p *indexedParses) remove(uri string) {
	for i := range p.entries {
		if p.entries[i].uri == uri {
			p.entries[i] = indexedParse{}
		}
	}
}

// IndexedParse returns the syntax tree and declarations of uri when its
// indexed text is source, so a request on the text just indexed doesn't parse
// it again. The index keeps the last few files it or a request parsed (see
// KeepParse); both results are shared and read-only.
func (idx *Index) IndexedParse(uri, source string) (phpparser.Parsed, *parser.FileNode, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.fileSource[uri].text != source {
		return phpparser.Parsed{}, nil, false
	}
	for _, entry := range idx.parses.entries {
		if entry.uri == uri && entry.text == source {
			return entry.parsed, entry.file, true
		}
	}
	return phpparser.Parsed{}, nil, false
}

// KeepParse keeps a request's parse of uri for IndexedParse when source is
// still uri's indexed text, parsed with GrammarOf(uri), so the next request
// on an unedited file reuses it.
func (idx *Index) KeepParse(uri, source string, parsed phpparser.Parsed, file *parser.FileNode) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if indexed, ok := idx.fileSource[uri]; !ok || indexed.text != source || indexed.ideHelper {
		return
	}
	idx.parses.put(indexedParse{uri: uri, text: source, parsed: parsed, file: file})
}
