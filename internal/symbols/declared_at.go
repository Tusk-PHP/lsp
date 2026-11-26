package symbols

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// MemberDeclaredAt returns the member whose declaration name in the file at
// uri covers pos and is word, or nil when pos isn't on one. Members PHP gives
// an enum point at the enum's name and are never declared there.
func (idx *Index) MemberDeclaredAt(uri string, pos textpos.Position, word string) *Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	for _, sym := range idx.fileSymbols[uri] {
		r := sym.Range
		if !sym.Kind.IsMember() || sym.URI != uri || r.Start.Line != pos.Line || r.Start.Character == r.End.Character {
			continue
		}
		if pos.Character < r.Start.Character || pos.Character > r.End.Character || !sym.isNamed(word) {
			continue
		}
		if owner := idx.symbols[sym.ParentFQN]; owner != nil && owner.URI == uri && owner.Range == r {
			continue
		}
		return sym
	}
	return nil
}

func (s *Symbol) isNamed(word string) bool {
	name, word := strings.TrimPrefix(s.Name, "$"), strings.TrimPrefix(word, "$")
	if s.Kind == KindMethod {
		return strings.EqualFold(name, word)
	}
	return name == word
}

func (idx *Index) DeclarationBefore(uri, source string, pos textpos.Position) (uint32, textpos.Position) {
	idx.mu.RLock()
	text := idx.fileSource[uri].text
	var last textpos.Position
	for _, sym := range idx.fileSymbols[uri] {
		if sym.URI != uri || sym.IsVirtual || !sym.Kind.declaresCode() {
			continue
		}
		if start := sym.Range.Start; positionBefore(start, pos) && positionBefore(last, start) {
			last = start
		}
	}
	idx.mu.RUnlock()
	offset, ok := offsetIn(text, last)
	if !ok || len(source) < int(offset) || text[:offset] != source[:offset] {
		return 0, textpos.Position{}
	}
	return offset, last
}

func (k SymbolKind) declaresCode() bool {
	switch k {
	case KindFunction, KindMethod, KindClass, KindInterface, KindTrait, KindEnum:
		return true
	}
	return false
}

func positionBefore(a, b textpos.Position) bool {
	return a.Line < b.Line || a.Line == b.Line && a.Character < b.Character
}

func offsetIn(text string, pos textpos.Position) (uint32, bool) {
	start := 0
	for range pos.Line {
		nl := strings.IndexByte(text[start:], '\n')
		if nl < 0 {
			return 0, false
		}
		start += nl + 1
	}
	offset := start + pos.Character
	if offset > len(text) {
		return 0, false
	}
	return uint32(offset), true
}
