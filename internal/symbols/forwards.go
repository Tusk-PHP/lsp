package symbols

// SetCallForwards records, for a framework, the classes that hand calls to
// members they don't have to another type, as Laravel's Eloquent Model does
// through __call and __callStatic. Keys are class FQNs; values are PHPDoc
// types, where `static` means the class the call is made on, as in
// `\Illuminate\Database\Eloquent\Builder<static>`. Subclasses inherit them.
// It's set once, before indexing; the map is kept as given and only read.
func (idx *Index) SetCallForwards(forwards map[string][]string) {
	idx.mu.Lock()
	idx.callForwards = forwards
	idx.mu.Unlock()
}

// CallForwards returns the types class fqn itself forwards calls to, or nil.
// The slice is read-only.
func (idx *Index) CallForwards(fqn string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.callForwards[fqn]
}

// ScopeConvention is how a query builder class finds the scopes of the model
// it's for, as Laravel's Eloquent builder does: a model method named Prefix
// plus the scope's name with its first letter in upper case
// (scopeActive() for ->active()), or a method carrying Attribute (Laravel
// 12's #[Scope]). The model is the builder's first type argument.
type ScopeConvention struct {
	Prefix    string
	Attribute string // attribute class FQN, or ""
}

// SetScopeConventions records the scope conventions of builder classes, by
// class FQN. Subclasses inherit them. It's set once, before indexing.
func (idx *Index) SetScopeConventions(conventions map[string]ScopeConvention) {
	idx.mu.Lock()
	idx.scopeConventions = conventions
	idx.mu.Unlock()
}

// ScopeConventionOf returns the scope convention class fqn itself declares.
func (idx *Index) ScopeConventionOf(fqn string) (ScopeConvention, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	c, ok := idx.scopeConventions[fqn]
	return c, ok
}

// SetCollectionFactories records the collection classes whose instances for
// an item class are built by a method of that class, as Laravel builds an
// Eloquent Collection<TKey, TModel> with TModel::newCollection(). Keys are
// collection class FQNs, values the factory method's name. It's set once,
// before indexing; the map is kept as given and only read.
func (idx *Index) SetCollectionFactories(factories map[string]string) {
	idx.mu.Lock()
	idx.collectionFactories = factories
	idx.mu.Unlock()
}

// CollectionFactory returns the name of the item class's method that builds
// collection class fqn, if it has one.
func (idx *Index) CollectionFactory(fqn string) (string, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	factory, ok := idx.collectionFactories[fqn]
	return factory, ok
}
