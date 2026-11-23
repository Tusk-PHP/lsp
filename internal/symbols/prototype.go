package symbols

import "strings"

func (idx *Index) Prototype(member *Symbol) *Symbol {
	if member == nil || member.ParentFQN == "" || !member.Kind.IsMember() {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.prototypeLocked(member.ParentFQN, member, map[string]bool{strings.ToLower(member.ParentFQN): true})
}

func (idx *Index) prototypeLocked(owner string, member *Symbol, visited map[string]bool) *Symbol {
	ancestors := idx.implementsMap[owner]
	if parent, ok := idx.inheritanceMap[owner]; ok {
		ancestors = append(ancestors[:len(ancestors):len(ancestors)], parent)
	}
	for _, ancestor := range ancestors {
		key := strings.ToLower(ancestor)
		if visited[key] {
			continue
		}
		visited[key] = true
		if higher := idx.prototypeLocked(ancestor, member, visited); higher != nil {
			return higher
		}
		if declared := idx.declaredMemberLocked(ancestor, member); declared != nil {
			return declared
		}
	}
	return nil
}

func (idx *Index) declaredMemberLocked(owner string, member *Symbol) *Symbol {
	fqn := owner + "::" + member.Name
	if sym := idx.symbols[fqn]; sym != nil && sym.Kind == member.Kind {
		return sym
	}
	ownerSym := idx.symbols[owner]
	if member.Kind != KindMethod || ownerSym == nil {
		return nil
	}
	for _, sym := range idx.fileSymbols[ownerSym.URI] {
		if sym.Kind == KindMethod && sym.ParentFQN == owner && strings.EqualFold(sym.Name, member.Name) {
			return sym
		}
	}
	return nil
}
