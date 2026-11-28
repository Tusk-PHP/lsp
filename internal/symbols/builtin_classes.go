package symbols

type builtinClassLike struct {
	Name                string
	Kind                SymbolKind
	IsAbstract          bool
	IsFinal             bool
	Extends             string
	Implements          []string
	VersionedImplements []builtinRelation
	Since               string
	Extension           string
	Members             []builtinMember
}

type builtinRelation struct {
	Name    string
	Since   string
	Removed string
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

var generatedBuiltinClassAvailability = builtinClassAvailability(generatedBuiltinClassLikes)

func builtinClassAvailability(classes []builtinClassLike) map[string]BuiltinAvailability {
	out := make(map[string]BuiltinAvailability)
	for _, c := range classes {
		if c.Since != "" || c.Extension != "" {
			out[c.Name] = BuiltinAvailability{Since: c.Since, Extension: c.Extension}
		}
	}
	return out
}

func (idx *Index) addBuiltinClassLikeWithMembersLocked(c builtinClassLike, profile BuiltinProfile) {
	if !builtinAvailable(c.Name, profile) {
		return
	}
	class := &Symbol{Name: shortName(c.Name), FQN: c.Name, Kind: c.Kind, Source: SourceBuiltin, URI: builtinRegistryURI,
		IsAbstract: c.IsAbstract, IsFinal: c.IsFinal, Extends: c.Extends, Implements: c.implementsIn(profile.PHPVersion)}
	hidden := idx.declaredOverBuiltinLocked(class)
	idx.storeBuiltinLocked(class)
	members := make([]Symbol, 0, len(c.Members))
	class.Children = make([]*Symbol, 0, len(c.Members))
	for _, m := range c.Members {
		if !builtinInVersion(m.Since, m.Removed, profile.PHPVersion) {
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

func (c builtinClassLike) implementsIn(version string) []string {
	if len(c.VersionedImplements) == 0 {
		return c.Implements
	}
	out := make([]string, len(c.Implements), len(c.Implements)+len(c.VersionedImplements))
	copy(out, c.Implements)
	for _, r := range c.VersionedImplements {
		if builtinInVersion(r.Since, r.Removed, version) {
			out = append(out, r.Name)
		}
	}
	return out
}

func builtinInVersion(since, removed, version string) bool {
	return (since == "" || compareBuiltinVersions(since, version) <= 0) &&
		(removed == "" || compareBuiltinVersions(removed, version) > 0)
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
