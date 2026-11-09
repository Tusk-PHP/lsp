package resolve

import (
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
)

// A variable's type is the same everywhere its last write still holds: on
// every line between `$builder = …` and the next write. Diagnostics ask about
// the same variable at each line it's used on, and each answer walks back
// through the file, so the resolver remembers answers per write while the
// file and the index stay as they are.

// varMemoKey identifies a variable's value: its binding, the write that gave
// the value, and whether that write certainly holds where it's read.
type varMemoKey struct {
	binding *scope.Binding
	rebind  int // index into binding.Rebinds, or -1 for its declaration
	holds   bool
}

// maxMemoDistance keeps the cache from changing answers: variable lookups
// look back a limited number of lines (200) for the assignment, so a read
// further from the write than that can't be answered like a nearby one.
const maxMemoDistance = 190

// varMemoKeyAt returns the key for varName read at pos, or false when its
// type might depend on more than its last write: no binding (a global or a
// captured variable), a read far from the write, or a call in between that
// may narrow it.
func (r *Resolver) varMemoKeyAt(varName string, lines []string, pos protocol.Position) (varMemoKey, bool) {
	doc := r.scopeDocument(lines)
	s := doc.ScopeAt(pos)
	if s == nil || s.Kind == scope.ScopeFile {
		return varMemoKey{}, false
	}
	var b *scope.Binding
	for _, cand := range s.Bindings {
		if cand.Name == varName {
			b = cand
			break
		}
	}
	if b == nil || b.Kind == scope.BindingImport {
		return varMemoKey{}, false
	}
	key := varMemoKey{binding: b, rebind: -1, holds: true}
	writeLine := b.Decl.Start.Line
	if b.Kind == scope.BindingParameter {
		writeLine = s.Range.Start.Line
	}
	if rb := b.LatestRebindBefore(pos); rb != nil {
		for i := range b.Rebinds {
			if &b.Rebinds[i] == rb {
				key.rebind = i
			}
		}
		key.holds = rb.HoldsAt(pos)
		writeLine = rb.At.Line
	}
	if pos.Line-writeLine > maxMemoDistance || pos.Line < writeLine {
		return varMemoKey{}, false
	}
	from := r.lastWriteLine(varName, lines, pos)
	if from >= 0 && from <= pos.Line && r.mayNarrow(lines, from, pos, varName) {
		return varMemoKey{}, false
	}
	return key, true
}

func (r *Resolver) varMemoGet(key varMemoKey) (ResolvedType, bool) {
	r.scopeMu.Lock()
	defer r.scopeMu.Unlock()
	if r.varMemoDoc != r.scopeDoc || r.Index == nil || r.varMemoGen != r.Index.Generation() {
		return ResolvedType{}, false
	}
	t, ok := r.varMemos[key]
	return t, ok
}

func (r *Resolver) varMemoPut(key varMemoKey, t ResolvedType) {
	r.scopeMu.Lock()
	defer r.scopeMu.Unlock()
	if r.scopeDoc == nil || key.binding == nil || r.Index == nil {
		return
	}
	gen := r.Index.Generation()
	if r.varMemoDoc != r.scopeDoc || r.varMemoGen != gen || len(r.varMemos) >= 8192 {
		r.varMemos = make(map[varMemoKey]ResolvedType)
		r.varMemoDoc, r.varMemoGen = r.scopeDoc, gen
	}
	r.varMemos[key] = t
}
