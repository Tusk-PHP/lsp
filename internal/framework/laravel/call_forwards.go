package laravel

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
