package laravel

import "github.com/Tusk-PHP/lsp/internal/symbols"

// CallForwards are the classes Laravel hands unknown calls on to another
// type, for symbols.Index.SetCallForwards. An Eloquent model forwards
// static and instance calls it doesn't define to a new query builder for
// itself (Model::__callStatic and __call), so Category::where() is
// Builder<Category>::where(), and the builder's own @mixin brings in the
// base query builder's methods.
func CallForwards() map[string][]string {
	return map[string][]string{
		"Illuminate\\Database\\Eloquent\\Model": {"\\Illuminate\\Database\\Eloquent\\Builder<static>"},
	}
}

// ScopeConventions are how Eloquent's query builder finds its model's local
// scopes, for symbols.Index.SetScopeConventions: scopeActive($query) or a
// method with Laravel 12's #[Scope] is called as ->active() on a
// Builder<Model>, and as Model::active() through the call forward above.
func ScopeConventions() map[string]symbols.ScopeConvention {
	return map[string]symbols.ScopeConvention{
		"Illuminate\\Database\\Eloquent\\Builder": {
			Prefix:    "scope",
			Attribute: "Illuminate\\Database\\Eloquent\\Attributes\\Scope",
		},
	}
}
