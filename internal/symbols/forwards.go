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
