package symbols

type builtinClassLike struct {
	Name       string
	Kind       SymbolKind
	IsAbstract bool
	IsFinal    bool
	Extends    string
	Implements []string
	Members    []builtinMember
}

type builtinMember struct {
	Name       string
	Kind       SymbolKind
	Visibility string
	IsStatic   bool
	IsAbstract bool
	IsFinal    bool
	IsReadonly bool
	Type       string
	Value      string
	ReturnType string
	Params     []ParamInfo
	Since      string
	Removed    string
}

func (idx *Index) addBuiltinClassLikeWithMembersLocked(c builtinClassLike, profile BuiltinProfile) {
	if !builtinAvailable(c.Name, profile) {
		return
	}
	class := &Symbol{Name: shortName(c.Name), FQN: c.Name, Kind: c.Kind, Source: SourceBuiltin, URI: builtinRegistryURI,
		IsAbstract: c.IsAbstract, IsFinal: c.IsFinal, Extends: c.Extends, Implements: c.Implements}
	hidden := idx.declaredOverBuiltinLocked(class)
	idx.storeBuiltinLocked(class)
	members := make([]Symbol, 0, len(c.Members))
	class.Children = make([]*Symbol, 0, len(c.Members))
	for _, m := range c.Members {
		if !m.availableIn(profile.PHPVersion) {
			continue
		}
		members = append(members, Symbol{Name: m.Name, FQN: c.Name + "::" + m.Name, Kind: m.Kind, Source: SourceBuiltin, URI: builtinRegistryURI,
			ParentFQN: c.Name, Visibility: m.Visibility, IsStatic: m.IsStatic, IsAbstract: m.IsAbstract, IsFinal: m.IsFinal,
			IsReadonly: m.IsReadonly, Type: m.Type, Value: m.Value, ReturnType: m.ReturnType, Params: m.Params})
		member := &members[len(members)-1]
		class.Children = append(class.Children, member)
		if hidden {
			idx.fileSymbols[member.URI] = append(idx.fileSymbols[member.URI], member)
			continue
		}
		idx.storeBuiltinLocked(member)
	}
	if !hidden {
		idx.linkTypeLocked(class)
	}
}

func (m builtinMember) availableIn(version string) bool {
	return (m.Since == "" || compareBuiltinVersions(m.Since, version) <= 0) &&
		(m.Removed == "" || compareBuiltinVersions(m.Removed, version) > 0)
}

func HasStubbedMembers(sym *Symbol) bool {
	if sym.URI == builtinRegistryURI {
		return false
	}
	if sym.Extends != "" || len(sym.Implements) > 0 {
		return true
	}
	for _, child := range sym.Children {
		if child.URI != builtinRegistryURI {
			return true
		}
	}
	return false
}
