package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// narrowInstanceof is what t becomes where `$x instanceof name` is value.
func (r *Resolver) narrowInstanceof(t ResolvedType, name string, value bool, file *parser.FileNode) ResolvedType {
	switch strings.ToLower(name) {
	case "self", "static", "parent":
		if value {
			return ResolvedType{} // which class that is isn't worked out here
		}
		return t
	}
	fqn := r.ResolveClassName(name, file)
	if fqn == "" || r.Index == nil || r.Index.Lookup(fqn) == nil {
		// A class that isn't indexed could have any member.
		if value {
			return ResolvedType{}
		}
		return t
	}
	cls := ResolvedType{FQN: fqn}
	t = splitWrittenUnion(t)
	if !value {
		return r.removeType(t, cls)
	}
	parts := t.Parts()
	if len(t.Intersection) > 0 {
		parts = t.Intersection
	}
	if t.IsEmpty() || len(parts) == 0 {
		return cls
	}
	var kept []ResolvedType
	narrower := false
	for _, p := range parts {
		switch {
		case p.FQN == "" || strings.EqualFold(p.FQN, "mixed") || strings.EqualFold(p.FQN, "object"):
			narrower = true
		case r.typeMatches(p, cls) == matchYes:
			kept = append(kept, p)
		case !IsBuiltinTypeName(p.FQN) && r.Index.IsSubtypeOf(fqn, p.FQN):
			narrower = true
		}
	}
	switch {
	case len(kept) > 0 && len(t.Intersection) == 0:
		return MakeUnion(kept, false)
	case len(kept) > 0:
		return t
	case narrower:
		return cls
	}
	// A class and an interface it doesn't implement, or two unrelated
	// classes: an object that is both, as a subclass may be.
	var objects []ResolvedType
	for _, p := range parts {
		if !IsBuiltinTypeName(p.FQN) {
			objects = append(objects, p)
		}
	}
	if len(objects) == 0 || len(objects) > 1 && len(t.Intersection) == 0 {
		return cls
	}
	return makeIntersection(append(objects, cls), false)
}

// splitWrittenUnion makes a union kept as written in its name, as a
// parameter's declared `Foo|Bar` is, into its parts.
func splitWrittenUnion(t ResolvedType) ResolvedType {
	if len(t.Union) > 0 || len(t.Intersection) > 0 || !strings.Contains(t.FQN, "|") || strings.ContainsAny(t.FQN, "<({&") {
		return t
	}
	var parts []ResolvedType
	nullable := t.Nullable
	for _, part := range strings.Split(t.FQN, "|") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "?")
		switch {
		case part == "":
		case strings.EqualFold(part, "null"):
			nullable = true
		default:
			parts = append(parts, ResolvedType{FQN: part})
		}
	}
	if len(parts) == 0 {
		return t
	}
	return MakeUnion(parts, nullable)
}
