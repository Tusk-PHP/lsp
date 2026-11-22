package resolve

import (
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A class can hand calls to members it doesn't have to another type, as
// Laravel's Eloquent Model does: Category::where() runs on a fresh
// Builder<Category>. A framework registers these with the index
// (symbols.Index.SetCallForwards). Unlike @mixin, the call's result is the
// forwarded type's, so a fluent builder method gives back the builder, not
// the model.

// ForwardTypes returns the types rt's class, or one of its ancestors,
// forwards calls to, with `static` as rt.
func (r *Resolver) ForwardTypes(rt ResolvedType) []ResolvedType {
	if r.Index == nil || rt.FQN == "" || IsBuiltinTypeName(rt.FQN) {
		return nil
	}
	var out []ResolvedType
	for _, cls := range append([]string{rt.FQN}, r.Index.GetInheritanceChain(rt.FQN)...) {
		for _, raw := range r.Index.CallForwards(cls) {
			env := typeEnv{static: rt, owner: cls}
			if t := r.applyEnv(r.parseType(raw), env); !t.IsEmpty() && !IsBuiltinTypeName(t.FQN) {
				out = append(out, t)
			}
		}
	}
	return out
}

// ForwardedReceiver returns the type a call of name on rt actually runs on
// when rt's class doesn't have the member, nor any @mixin it names, but
// forwards it: Builder<Category> for Category::where(). ok is false when rt
// has the member itself or nothing it forwards to does.
func (r *Resolver) ForwardedReceiver(rt ResolvedType, name string, call bool) (ResolvedType, bool) {
	if m, _ := r.FindMemberOn(rt, name, call); m != nil {
		return ResolvedType{}, false
	}
	for _, fwd := range r.ForwardTypes(rt) {
		if m, _ := r.FindMemberOn(fwd, name, call); m != nil {
			return fwd, true
		}
	}
	return ResolvedType{}, false
}

// LookupMember finds the member name for a reader such as hover or go to
// definition: on rt's class, then the @mixin types it names, then the types
// it forwards calls to. owner is the type it was found on.
func (r *Resolver) LookupMember(rt ResolvedType, name string, call bool) (*symbols.Symbol, ResolvedType) {
	if m, owner := r.FindMemberOn(rt, name, call); m != nil {
		return m, owner
	}
	for _, fwd := range r.ForwardTypes(rt) {
		if m, owner := r.FindMemberOn(fwd, name, call); m != nil {
			return m, owner
		}
	}
	return nil, ResolvedType{}
}

// FindMemberAnywhere is FindMember for readers that only have a class name,
// such as hover and go to definition: when the class doesn't declare the
// member, it's looked up on the @mixin types and on what the class forwards
// calls to, so `$query->orderBy()` on an Eloquent builder and
// `Category::find()` both reach their method.
func (r *Resolver) FindMemberAnywhere(classFQN, name string) *symbols.Symbol {
	if m := r.FindMember(classFQN, name); m != nil {
		return m
	}
	if r.Index == nil || classFQN == "" {
		return nil
	}
	rt := ResolvedType{FQN: classFQN}
	if m, _ := r.LookupMember(rt, name, true); m != nil {
		return m
	}
	m, _ := r.LookupMember(rt, name, false)
	return m
}
