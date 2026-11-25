package symbols

import "strings"

// MemberFamily returns the declarations that share member's name across its
// hierarchy and must keep it together: member, the members of its ancestors
// it overrides or implements, and every override or implementation below any
// of those. A private member is overridden by nothing and overrides nothing,
// so it is its own family, as is a symbol that isn't a class member.
func (idx *Index) MemberFamily(member *Symbol) []*Symbol {
	if member == nil {
		return nil
	}
	if member.ParentFQN == "" || !member.Kind.IsMember() || member.Visibility == "private" {
		return []*Symbol{member}
	}
	idx.ensureClass(member.ParentFQN)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	family := []*Symbol{member}
	inFamily := map[*Symbol]bool{member: true}
	add := func(sym *Symbol) bool {
		if sym == nil || inFamily[sym] || sym.Visibility == "private" {
			return false
		}
		inFamily[sym] = true
		family = append(family, sym)
		return true
	}
	roots := []string{member.ParentFQN}
	ancestors := []string{member.ParentFQN}
	seenUp := map[string]bool{strings.ToLower(member.ParentFQN): true}
	for i := 0; i < len(ancestors); i++ {
		idx.walkAncestors(ancestors[i], func(next string, _ bool) {
			key := strings.ToLower(next)
			if seenUp[key] {
				return
			}
			seenUp[key] = true
			ancestors = append(ancestors, next)
			if add(idx.declaredMemberLocked(next, member)) {
				roots = append(roots, next)
			}
		})
	}
	seenDown := map[string]bool{}
	for len(roots) > 0 {
		owner := roots[0]
		roots = roots[1:]
		idx.directSubtypesLocked(owner, func(names []string) {
			for _, name := range names {
				key := strings.ToLower(name)
				if seenDown[key] {
					continue
				}
				seenDown[key] = true
				add(idx.declaredMemberLocked(name, member))
				roots = append(roots, name)
			}
		})
	}
	return family
}
