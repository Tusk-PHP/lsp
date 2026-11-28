package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestTemplateMappingsTypeTheReturnsOfEloquentAndCollections(t *testing.T) {
	idx := symbols.NewIndex()
	idx.SetTemplateMappings(TemplateMappings())
	idx.IndexFile("file:///vendor/Eloquent.php", `<?php
namespace Illuminate\Database\Eloquent;
class Builder {
    public function get() {}
    public function first() {}
    public function firstOrFail() {}
    public function where() {}
}
class Collection {
    public function first() {}
    public function map() {}
    public function count() {}
}
`)
	idx.IndexFile("file:///vendor/Support.php", `<?php
namespace Illuminate\Support;
class Collection {
    public function last() {}
    public function keys() {}
}
`)

	category := resolve.ResolvedType{FQN: `App\Models\Category`}
	key := resolve.ResolvedType{FQN: "int"}
	builderOf := []resolve.ResolvedType{category}
	collectionOf := []resolve.ResolvedType{key, category}
	r := resolve.NewResolver(idx)

	cases := []struct {
		member  string
		context []resolve.ResolvedType
		want    string
	}{
		{EloquentBuilder + "::get", builderOf, eloquentCollection + `<int, App\Models\Category>`},
		{EloquentBuilder + "::first", builderOf, `?App\Models\Category`},
		{EloquentBuilder + "::firstOrFail", builderOf, `App\Models\Category`},
		{EloquentBuilder + "::where", builderOf, EloquentBuilder + `<App\Models\Category>`},
		{eloquentCollection + "::first", collectionOf, `?App\Models\Category`},
		{eloquentCollection + "::map", collectionOf, supportCollection + "<int, mixed>"},
		{eloquentCollection + "::count", collectionOf, "int"},
		{supportCollection + "::last", collectionOf, `?App\Models\Category`},
		{supportCollection + "::keys", collectionOf, supportCollection + `<int, App\Models\Category>`},
	}
	for _, tc := range cases {
		member := idx.Lookup(tc.member)
		if member == nil {
			t.Fatalf("%s is not indexed", tc.member)
		}
		if got := r.MemberTypeResolved(member, nil, "", tc.context).String(); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.member, got, tc.want)
		}
	}
}
