package symbols

// Prototype returns the topmost declaration of member: the interface, parent
// class or trait member it implements or overrides, as PHP sees it. A parent
// class's private members aren't inherited, and a constructor's prototype is
// only an interface's or an abstract one. receiver is the class member was
// reached through, so a trait method a class uses finds the interface the
// class implements; when receiver doesn't extend member's class, the walk
// starts at member's class. A trait an insteadof rule leaves out declares
// nothing, and a method a class or trait exposes under an `as` alias is
// declared there as the method it aliases. Builtin declarations have no
// location and are passed over, so the project's own declaration below one wins. It returns nil
// when nothing above declares member.
func (idx *Index) Prototype(receiver string, member *Symbol) *Symbol {
	if member == nil || member.ParentFQN == "" || !member.Kind.IsMember() {
		return nil
	}
	idx.ensureClass(receiver)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	start := member.ParentFQN
	if receiver != "" && idx.isSubtypeOfLocked(receiver, start) {
		start = receiver
	}
	w := prototypeWalk{idx: idx, member: member, visited: map[string]bool{FoldName(start): true}}
	return w.above(start, false)
}

type prototypeWalk struct {
	idx     *Index
	member  *Symbol
	visited map[string]bool
}

func (w *prototypeWalk) above(owner string, inherited bool) *Symbol {
	for _, iface := range w.idx.implementsMap[owner] {
		if found := w.visit(iface, true); found != nil {
			return found
		}
	}
	if parent, ok := w.idx.inheritanceMap[owner]; ok {
		if found := w.visit(parent, true); found != nil {
			return found
		}
	}
	traits := w.idx.traitMap[owner]
	var rules *traitRules
	if len(traits) > 0 && w.member.Kind == KindMethod {
		rules = w.idx.traitRulesLocked(owner)
	}
	for _, trait := range traits {
		if rules.excludes(trait, w.member.Name) {
			continue
		}
		if found := w.visit(trait, inherited); found != nil {
			return found
		}
	}
	return nil
}

func (w *prototypeWalk) visit(ancestor string, inherited bool) *Symbol {
	key := FoldName(ancestor)
	if w.visited[key] {
		return nil
	}
	w.visited[key] = true
	if higher := w.above(ancestor, inherited); higher != nil {
		return higher
	}
	return w.declared(ancestor, inherited)
}

func (w *prototypeWalk) declared(ancestor string, inherited bool) *Symbol {
	sym := w.idx.declaredMemberLocked(ancestor, w.member)
	switch {
	case sym == nil, IsBuiltin(sym), EqualFold(sym.ParentFQN, w.member.ParentFQN):
		return nil
	case inherited && sym.Visibility == "private":
		return nil
	case w.member.Kind == KindMethod && EqualFold(w.member.Name, "__construct") && !sym.IsAbstract:
		if owner := w.idx.symbols[ancestor]; owner == nil || owner.Kind != KindInterface {
			return nil
		}
	}
	return sym
}

func (idx *Index) declaredMemberLocked(owner string, member *Symbol) *Symbol {
	if sym := idx.symbols[owner+"::"+member.Name]; sym != nil && sym.Kind == member.Kind {
		return sym
	}
	ownerSym := idx.symbols[owner]
	if member.Kind != KindMethod || ownerSym == nil {
		return nil
	}
	if sym := methodNamed(ownerSym.Children, member.Name); sym != nil {
		return sym
	}
	return idx.aliasedMethodLocked(ownerSym, member.Name)
}

func methodNamed(members []*Symbol, name string) *Symbol {
	for _, sym := range members {
		if sym.Kind == KindMethod && EqualFold(sym.Name, name) {
			return sym
		}
	}
	return nil
}
