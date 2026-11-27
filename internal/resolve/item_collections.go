package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func (r *Resolver) itemCollection(t ResolvedType) ResolvedType {
	if r.Index == nil || len(t.Params) == 0 || t.FQN == "" {
		return t
	}
	factory, ok := r.Index.CollectionFactory(t.FQN)
	if !ok {
		return t
	}
	item := t.Params[len(t.Params)-1].FQN
	if item == "" || IsBuiltinTypeName(item) {
		return t
	}
	if class := r.builtCollection(item, factory, t.FQN); class != "" {
		t.FQN = class
	}
	return t
}

func (r *Resolver) builtCollection(item, factory, base string) string {
	var method *symbols.Symbol
	for _, m := range r.Index.ClassMembersNamed(item, factory) {
		if m.Kind == symbols.KindMethod {
			method = m
			break
		}
	}
	if method == nil {
		return ""
	}
	class := r.parseType(r.rawMemberReturnType(method)).FQN
	if class == "" || IsBuiltinTypeName(class) {
		return ""
	}
	class = strings.TrimPrefix(r.qualifyDocName(class, method.ParentFQN, nil), "\\")
	if strings.EqualFold(class, base) {
		return ""
	}
	for _, ancestor := range r.Index.GetInheritanceChain(class) {
		if strings.EqualFold(ancestor, base) {
			return class
		}
	}
	return ""
}
