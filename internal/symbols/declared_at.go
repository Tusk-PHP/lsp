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
		return EqualFold(name, word)
	}
	return name == word
}
