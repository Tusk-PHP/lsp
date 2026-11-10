package resolve

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
)

// A local variable's type at a read is the type of the writes that can reach
// it (scope.ReachingWrites), each typed where it is written: its right-hand
// side reads other variables as they are there, not at the read.

// reachingType answers the type of a local variable from the writes that
// reach the read. ok is false when the answer is left to the line scan from
// the read: there's no write before it, or the one write that reaches it can't
// be typed on its own and isn't an assignment the scan would read; but never
// for a read in a write's own right-hand side.
func (r *Resolver) reachingType(varName string, lines []string, pos protocol.Position, file *parser.FileNode, vs variableScope) (ResolvedType, bool) {
	b := vs.binding
	if b == nil || b.Kind == scope.BindingImport {
		return ResolvedType{}, false
	}
	reach := vs.reach
	switch {
	case reach.OnWrite:
		return r.writeValueType(varName, lines, file, b, reach.Writes[0]), true
	case reach.TooMany:
		return ResolvedType{}, true
	case !reach.Single():
		rt := r.reachingValuesType(varName, lines, vs.readPos, file, vs)
		return r.narrowVariable(varName, rt, lines, pos, file), true
	}
	// In the right-hand side of a write, the line scan from the read would
	// find that write and give the value it's about to assign.
	inRHS := vs.inRHS
	if reach.Latest == scope.NoWrite {
		return ResolvedType{}, inRHS
	}
	rt := r.writeValueType(varName, lines, file, b, reach.Latest)
	if rt.IsEmpty() && !rt.Nullable {
		// The line scan would read the same assignment, only with the wrong
		// values for the variables on its right.
		return ResolvedType{}, inRHS || readableWrite(lines, b, reach.Latest)
	}
	return r.narrowVariable(varName, rt, lines, pos, file), true
}

// reachingValuesType is the type of a variable that may hold the value of any
// of several writes, as after `if ($c) { $x = new Bar(); }`: the union of what
// each of them gives. When any of them can't be typed, neither can the
// variable: its type is left unknown rather than guessed from the others.
func (r *Resolver) reachingValuesType(varName string, lines []string, pos protocol.Position, file *parser.FileNode, vs variableScope) ResolvedType {
	if vs.reach.TooMany {
		return ResolvedType{}
	}
	b := vs.binding
	parts := make([]ResolvedType, 0, len(vs.reach.Writes))
	for _, w := range vs.reach.Writes {
		// A read in the right-hand side of a write in a loop, which reaches it
		// through the next iteration, while that write's value is being worked
		// out: it's left out here and checked against what's found (see
		// feedsItself). That cycle is expected, so it isn't counted as a cut.
		site := b.Site(w)
		inOwnRHS := !positionBefore(pos, site.RHS) && positionBefore(pos, site.End)
		rt, cut := r.writeValue(varName, lines, file, b, w, !inOwnRHS)
		if cut && inOwnRHS {
			continue
		}
		if rt.IsEmpty() && !rt.Nullable {
			return ResolvedType{}
		}
		parts = append(parts, rt)
	}
	return MakeUnion(parts, false)
}

// readableWrite reports whether write w is an assignment its value can be
// read from, so the line scan from a read would find nothing else.
func readableWrite(lines []string, b *scope.Binding, w int) bool {
	site := b.Site(w)
	if site.RHS == site.End || w == scope.DeclWrite && b.Kind == scope.BindingParameter {
		return false
	}
	_, _, ok := writeScope(b, w, &site).assignmentValue(lines, site.At.Line, b.Name)
	return ok
}

// PlainTypeName is a type as ResolveVariableType gives it: a class name
// without type arguments or a callable signature, so it can be looked up as
// it is; for a union, every part so, as "App\Foo|App\Bar|null". Every part is
// kept, since a reader that checked members against one of them would report
// what the others have.
func PlainTypeName(rt ResolvedType) string {
	if len(rt.Union) == 0 {
		return rt.FQN
	}
	names := make([]string, 0, len(rt.Union)+1)
	for _, part := range rt.Union {
		if part.FQN != "" && !slices.Contains(names, part.FQN) {
			names = append(names, part.FQN)
		}
	}
	if rt.Nullable {
		names = append(names, "null")
	}
	return strings.Join(names, "|")
}

func writeKey(b *scope.Binding, w int) string {
	return "write|" + b.Name + "|" + strconv.Itoa(b.Decl.Start.Line) + ":" + strconv.Itoa(b.Decl.Start.Character) + "|" + strconv.Itoa(w)
}

// writeValueType is the type of the value write w gives b, read from the
// write itself, remembered per write.
func (r *Resolver) writeValueType(varName string, lines []string, file *parser.FileNode, b *scope.Binding, w int) ResolvedType {
	rt, _ := r.writeValue(varName, lines, file, b, w, true)
	return rt
}

// writeValue is writeValueType, and reports whether it was cut short because
// that write's value is already being worked out further up. counted says
// whether that counts as a guard cut.
func (r *Resolver) writeValue(varName string, lines []string, file *parser.FileNode, b *scope.Binding, w int, counted bool) (ResolvedType, bool) {
	key := varMemoKey{binding: b, rebind: w, holds: true, onWrite: true}
	if rt, ok := r.varMemoGet(key); ok {
		return rt, false
	}
	exit, ok := r.enterGuard(writeKey(b, w), counted)
	if !ok {
		return ResolvedType{}, true
	}
	defer exit()
	cutsBefore := r.guardCuts.Load()

	if caught := b.Caught(w); len(caught) > 0 {
		rt := r.caughtType(caught, file)
		r.varMemoPut(key, rt)
		return rt, false
	}
	site := b.Site(w)
	pos := site.RHS
	if w == scope.DeclWrite && b.Kind == scope.BindingParameter {
		pos = b.Decl.End
	}
	vs := writeScope(b, w, &site)
	rt := r.resolveVariableTypeTypedBase(varName, lines, pos, file, vs)
	if !rt.IsEmpty() && b.ReadsItself(w) {
		rt = r.feedsItself(varName, lines, file, b, w, pos, rt)
	}
	if r.guardCuts.Load() == cutsBefore {
		r.varMemoPut(key, rt)
	}
	return rt, false
}

// caughtType is the value of a catch variable: the class it catches, or the
// union of them for `catch (A|B $e)`, named as the file's imports and
// namespace say.
func (r *Resolver) caughtType(names []string, file *parser.FileNode) ResolvedType {
	parts := make([]ResolvedType, 0, len(names))
	for _, name := range names {
		parts = append(parts, ResolvedType{FQN: r.ResolveClassName(name, file)})
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return MakeUnion(parts, false)
}

// feedsItself checks the value t of a write that reads the variable it
// assigns, as `$x = $x->next()` does. In a loop, that write reaches its own
// right-hand side through the next iteration, and t was worked out from the
// other values that reach it there. A value that names no class stands
// whatever those are: `$word = trim($word)` is a string, and calling a method
// on a scalar or an array gives no class for the next iteration. One that
// names a class stands only if those values are known and name it too, so the
// next iteration can't give another: `$q = $q->where()` on a query builder
// stays the builder. Otherwise its value isn't known.
func (r *Resolver) feedsItself(varName string, lines []string, file *parser.FileNode, b *scope.Binding, w int, pos protocol.Position, t ResolvedType) ResolvedType {
	classes := namedClasses(t)
	if len(classes) == 0 {
		return t
	}
	reach := r.scopeDocument(lines).ReachingWrites(b, pos)
	if reach.TooMany {
		return ResolvedType{}
	}
	if !slices.Contains(reach.Writes, w) {
		return t
	}
	var others []string
	for _, o := range reach.Writes {
		if o == w {
			continue
		}
		rt := r.writeValueType(varName, lines, file, b, o)
		if rt.IsEmpty() && !rt.Nullable {
			return ResolvedType{}
		}
		others = append(others, namedClasses(rt)...)
	}
	for _, c := range classes {
		if !slices.Contains(others, c) {
			return ResolvedType{}
		}
	}
	return t
}

// namedClasses returns the classes a type names, also those of a union kept
// as written in its name, as a builtin function's "string|array" is.
func namedClasses(rt ResolvedType) []string {
	var out []string
	for _, name := range rt.ClassNames() {
		for _, part := range strings.Split(name, "|") {
			part = strings.TrimPrefix(strings.TrimSpace(part), "?")
			if part != "" && !IsBuiltinTypeName(part) {
				out = append(out, part)
			}
		}
	}
	return out
}

func positionBefore(a, b protocol.Position) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Character < b.Character)
}
