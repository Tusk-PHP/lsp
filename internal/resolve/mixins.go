package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// maxMixinDepth bounds how far @mixin is followed through mixins of mixins.
const maxMixinDepth = 3

// MixinTypes returns the types a class's @mixin tags (and its parents') make
// its members available from, with the class's type arguments substituted:
// `@template T @mixin T` on Delegatee<Logger> gives Logger.
func (r *Resolver) MixinTypes(rt ResolvedType) []ResolvedType {
	if r.Index == nil || rt.FQN == "" || IsBuiltinTypeName(rt.FQN) {
		return nil
	}
	classes := append([]string{rt.FQN}, r.Index.GetInheritanceChain(rt.FQN)...)
	var out []ResolvedType
	for _, cls := range classes {
		sym := r.Index.Lookup(cls)
		if sym == nil || !strings.Contains(sym.DocComment, "mixin") {
			continue
		}
		doc := r.parseDoc(sym.DocComment)
		if doc == nil || len(doc.Mixins) == 0 {
			continue
		}
		subst := r.classSubst(cls, rt.FQN, rt.Params)
		subst = r.fillUnbound(subst, sym.Templates, cls, nil)
		env := typeEnv{subst: subst, static: rt, owner: cls}
		for _, m := range doc.Mixins {
			if t := r.applyEnv(r.parseType(m), env); !t.IsEmpty() && t.FQN != "mixed" && !IsBuiltinTypeName(t.FQN) {
				out = append(out, t)
			}
		}
	}
	return out
}

// FindMemberOn looks a member up on a value of type rt: on its classes
// first, then on what its @mixin tags bring in. owner is the type the member
// was found on, which is rt or one of its mixins.
func (r *Resolver) FindMemberOn(rt ResolvedType, name string, call bool) (member *symbols.Symbol, owner ResolvedType) {
	if m := r.FindAccessedMember(rt.MemberOwner(), name, call); m != nil {
		return m, rt
	}
	return r.findMixinMember(rt, name, call, 0)
}

func (r *Resolver) findMixinMember(rt ResolvedType, name string, call bool, depth int) (*symbols.Symbol, ResolvedType) {
	if depth >= maxMixinDepth {
		return nil, ResolvedType{}
	}
	for _, mixin := range r.MixinTypes(rt) {
		if m := r.FindAccessedMember(mixin.MemberOwner(), name, call); m != nil {
			return m, mixin
		}
		if m, owner := r.findMixinMember(mixin, name, call, depth+1); m != nil {
			return m, owner
		}
	}
	return nil, ResolvedType{}
}

// ClassStringTarget returns the class a class-string<T> value names, for
// `new $class` and `$class::create()`, or rt itself when it isn't one.
func ClassStringTarget(rt ResolvedType) ResolvedType {
	if rt.FQN == "class-string" && len(rt.Params) == 1 && !rt.Params[0].IsEmpty() {
		return rt.Params[0]
	}
	return rt
}
