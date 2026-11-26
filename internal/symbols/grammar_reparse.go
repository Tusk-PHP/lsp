package symbols

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
)

const reparseBatch = 256

type reparseCandidate struct {
	uri     string
	indexed indexedText
}

type reparsedFile struct {
	reparseCandidate
	file *parser.FileNode
}

func (t indexedText) mayRegrammar(grammars phpparser.GrammarRange) bool {
	if t.from == SourceBuiltin {
		return false
	}
	return !grammars.Admits(t.grammar) || !t.clean && t.grammar != grammars.Newest()
}

func (idx *Index) ReparseForGrammars(reparsed func(uri, source string)) bool {
	epoch, grammars, candidates := idx.reparseCandidates()
	for start := 0; start < len(candidates); start += reparseBatch {
		batch := reparse(candidates[start:min(start+reparseBatch, len(candidates))], grammars)
		swapped, current := idx.swapReparsed(epoch, batch)
		if !current {
			return false
		}
		for _, f := range swapped {
			reparsed(f.uri, f.indexed.text)
		}
	}
	return true
}

func (idx *Index) reparseCandidates() (uint64, phpparser.GrammarRange, []reparseCandidate) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var candidates []reparseCandidate
	for uri, indexed := range idx.fileSource {
		if indexed.mayRegrammar(idx.grammars) {
			candidates = append(candidates, reparseCandidate{uri: uri, indexed: indexed})
		}
	}
	slices.SortFunc(candidates, func(a, b reparseCandidate) int { return strings.Compare(a.uri, b.uri) })
	return idx.grammarEpoch, idx.grammars, candidates
}

func reparse(candidates []reparseCandidate, grammars phpparser.GrammarRange) []reparsedFile {
	out := make([]reparsedFile, 0, len(candidates))
	for _, c := range candidates {
		file := parser.ParseFileIn(c.indexed.text, grammars.After(c.indexed.grammar))
		if file.Grammar != c.indexed.grammar {
			out = append(out, reparsedFile{reparseCandidate: c, file: file})
		}
	}
	return out
}

func (idx *Index) swapReparsed(epoch uint64, batch []reparsedFile) ([]reparsedFile, bool) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.grammarEpoch != epoch {
		return nil, false
	}
	swapped := batch[:0]
	for _, f := range batch {
		if idx.fileSource[f.uri] != f.indexed {
			continue
		}
		if f.indexed.ideHelper {
			idx.indexIDEHelperParsedLocked(f.uri, f.indexed.text, f.file)
		} else {
			idx.replaceParsedLocked(f.uri, f.indexed.text, f.indexed.from, f.file)
		}
		swapped = append(swapped, f)
	}
	return swapped, true
}
