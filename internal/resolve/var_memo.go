package resolve

import (
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// A variable's type is the same everywhere its last write still holds: on
// every line between `$builder = …` and the next write. Diagnostics ask about
// the same variable at each line it's used on, and each answer walks back
// through the file, so the resolver remembers answers per write while the
// file and the index stay as they are.

// varMemoKey identifies a variable's value: its binding, the write that gave
// the value, whether that write certainly holds where it's read, and whether
// the read is on the write itself, whose value is read from its line alone.
type varMemoKey struct {
	binding *scope.Binding
	rebind  int // index into binding.Rebinds, or -1 for its declaration
	holds   bool
	onWrite bool
}

// maxMemoDistance keeps the cache from changing answers: variable lookups
// look back a limited number of lines (200) for the assignment, so a read
// further from the write than that can't be answered like a nearby one.
const maxMemoDistance = 190

// varMemoKeyAt returns the key for varName read at pos, or false when its
// type might depend on more than its last write: no binding (a global or a
// captured variable), several writes that may give its value, a read far from
// the write, or a call in between that may narrow it.
func (r *Resolver) varMemoKeyAt(vs variableScope, varName string, lines []string, pos textpos.Position) (varMemoKey, bool) {
	b := vs.binding
	if b == nil || b.Kind == scope.BindingImport || !vs.reach.Single() {
		return varMemoKey{}, false
	}
	key := varMemoKey{binding: b, rebind: -1, holds: true, onWrite: vs.reach.OnWrite}
	writeLine := b.Decl.Start.Line
	if b.Kind == scope.BindingParameter {
		writeLine = b.Scope.Range.Start.Line
	}
	if i := vs.reach.Latest; i >= 0 {
		rb := &b.Rebinds[i]
		key.rebind = i
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

func (r *Resolver) varMemoGet(lines []string, key varMemoKey) (ResolvedType, bool) {
	f := r.fileOfLines(lines)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.varMemos.get(r.Index.Generation(), key)
}

func (r *Resolver) varMemoPut(lines []string, key varMemoKey, t ResolvedType) {
	f := r.fileOfLines(lines)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.varMemos.put(r.Index.Generation(), key, t, maxVarMemos)
}
