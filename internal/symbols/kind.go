package symbols

func (k SymbolKind) IsClassLike() bool {
	return k == KindClass || k == KindInterface || k == KindTrait || k == KindEnum
}

func (s *Symbol) IsClassLike() bool {
	return s != nil && s.Kind.IsClassLike()
}

// IsMember reports whether the kind belongs to a class-like (method, property,
// constant or enum case) rather than standing on its own.
func (k SymbolKind) IsMember() bool {
	return k == KindMethod || k == KindProperty || k == KindConstant || k == KindEnumCase
}

func (s *Symbol) isGlobal() bool {
	return !s.Kind.IsMember() || s.Kind == KindConstant && s.ParentFQN == ""
}
