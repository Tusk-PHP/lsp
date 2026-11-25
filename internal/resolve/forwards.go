package resolve

import (
	"strings"

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
		// A #[Scope] method is declared on the model under the name it's
		// called by, but it's protected: Category::active() still goes
		// through the builder.
		if call {
			for _, fwd := range r.ForwardTypes(rt) {
				if r.ScopeMethod(fwd, name) == m {
					return fwd, true
				}
			}
		}
		return ResolvedType{}, false
	}
	for _, fwd := range r.ForwardTypes(rt) {
		if m, _ := r.FindMemberOn(fwd, name, call); m != nil {
			return fwd, true
		}
		if call && r.ScopeMethod(fwd, name) != nil {
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
	if call {
		if m := r.ScopeMethod(rt, name); m != nil {
			return m, rt
		}
	}
	for _, fwd := range r.ForwardTypes(rt) {
		if m, owner := r.FindMemberOn(fwd, name, call); m != nil {
			return m, owner
		}
		if call {
			if m := r.ScopeMethod(fwd, name); m != nil {
				return m, fwd
			}
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
	return r.FindMemberAnywhereOn(ResolvedType{FQN: classFQN}, name)
}

// AccessedMemberOn finds the member an access names on its receiver: the
// method for a call and the property otherwise, then the same name through
// the class's mixins and forwards, with the receiver's type arguments.
func (r *Resolver) AccessedMemberOn(owner ResolvedType, name string, call bool) *symbols.Symbol {
	if m := r.FindAccessedMember(owner.BaseFQN(), name, call); m != nil {
		return m
	}
	if m := r.FindMemberAnywhere(owner.BaseFQN(), name); m != nil {
		return m
	}
	return r.FindMemberAnywhereOn(owner, name)
}

// FindMemberAnywhereOn is FindMemberAnywhere on a type with its type
// arguments, so a scope called on Builder<Category> finds Category's.
func (r *Resolver) FindMemberAnywhereOn(rt ResolvedType, name string) *symbols.Symbol {
	if m := r.FindMember(rt.BaseFQN(), name); m != nil {
		// A #[Scope] method is reached through the builder, but it's the
		// same method: show it either way.
		return m
	}
	if r.Index == nil || rt.FQN == "" {
		return nil
	}
	if m, _ := r.LookupMember(rt, name, true); m != nil {
		return m
	}
	m, _ := r.LookupMember(rt, name, false)
	return m
}

// Local scopes: a query builder with a scope convention (see
// symbols.ScopeConvention) runs ->active() as its model's scopeActive() or
// #[Scope] active() method, and gives back the builder.

// ScopeMethod returns the model method a call of name on the builder type rt
// runs as a scope, or nil when rt has no scope convention, no model type
// argument, or no such scope.
func (r *Resolver) ScopeMethod(rt ResolvedType, name string) *symbols.Symbol {
	conv, model, ok := r.scopeConvention(rt)
	if !ok || name == "" {
		return nil
	}
	if conv.Prefix != "" {
		if m := r.FindAccessedMember(model, conv.Prefix+strings.ToUpper(name[:1])+name[1:], true); isScope(m, conv, true) {
			return m
		}
	}
	if conv.Attribute != "" {
		if m := r.FindAccessedMember(model, name, true); isScope(m, conv, false) {
			return m
		}
	}
	return nil
}

// Scope is a local scope a builder type can call: its name and the model
// method that implements it.
type Scope struct {
	Name   string
	Method *symbols.Symbol
}

// Scopes lists the local scopes the builder type rt can call, for
// completion.
func (r *Resolver) Scopes(rt ResolvedType) []Scope {
	conv, model, ok := r.scopeConvention(rt)
	if !ok {
		return nil
	}
	var out []Scope
	seen := map[string]bool{}
	for _, m := range r.Index.GetClassMembers(model) {
		name := ""
		switch {
		case conv.Prefix != "" && len(m.Name) > len(conv.Prefix) && strings.HasPrefix(m.Name, conv.Prefix) && isScope(m, conv, true):
			rest := m.Name[len(conv.Prefix):]
			name = strings.ToLower(rest[:1]) + rest[1:]
		case isScope(m, conv, false):
			name = m.Name
		}
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, Scope{Name: name, Method: m})
		}
	}
	return out
}

// scopeConvention finds rt's scope convention, on its class or an ancestor,
// and the model class it's for: rt's first type argument.
func (r *Resolver) scopeConvention(rt ResolvedType) (symbols.ScopeConvention, string, bool) {
	if r.Index == nil || rt.FQN == "" || len(rt.Params) == 0 || IsBuiltinTypeName(rt.FQN) {
		return symbols.ScopeConvention{}, "", false
	}
	model := rt.Params[0].BaseFQN()
	if model == "" || IsBuiltinTypeName(model) {
		return symbols.ScopeConvention{}, "", false
	}
	for _, cls := range append([]string{rt.FQN}, r.Index.GetInheritanceChain(rt.FQN)...) {
		if conv, ok := r.Index.ScopeConventionOf(cls); ok {
			return conv, model, true
		}
	}
	return symbols.ScopeConvention{}, "", false
}

// isScope reports whether m is a scope method by the convention: an instance
// method named with the prefix, or one with the scope attribute.
func isScope(m *symbols.Symbol, conv symbols.ScopeConvention, byPrefix bool) bool {
	if m == nil || m.Kind != symbols.KindMethod || m.IsStatic {
		return false
	}
	if byPrefix {
		return true
	}
	return conv.Attribute != "" && m.Attribute(conv.Attribute) != nil
}
