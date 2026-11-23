package symbols

func (k SymbolKind) IsClassLike() bool {
	return k == KindClass || k == KindInterface || k == KindTrait || k == KindEnum
}

func (s *Symbol) IsClassLike() bool {
	return s != nil && s.Kind.IsClassLike()
}
