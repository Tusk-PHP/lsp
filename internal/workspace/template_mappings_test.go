package workspace

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestNewMapsTheTemplatesOfBuildersWithoutGenericPHPDoc(t *testing.T) {
	for _, framework := range []string{"laravel", "none"} {
		ws := New(Options{RootPath: t.TempDir(), Framework: framework, BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
		ws.Index.IndexFile("file:///vendor/Builder.php", `<?php
namespace Illuminate\Database\Eloquent;
class Builder {
    public function first() {}
    public function get() {}
}
`)
		first := ws.Index.Lookup(`Illuminate\Database\Eloquent\Builder::first`)
		get := ws.Index.Lookup(`Illuminate\Database\Eloquent\Builder::get`)
		if first == nil || get == nil {
			t.Fatalf("%s: the builder's methods aren't indexed", framework)
		}
		r := resolve.NewResolver(ws.Index)
		model := []resolve.ResolvedType{{FQN: `App\Models\Category`}}
		if got := r.MemberTypeResolved(first, nil, "", model).String(); got != `?App\Models\Category` {
			t.Errorf("%s: Builder<Category>::first() = %q, want ?App\\Models\\Category", framework, got)
		}
		if got := r.MemberTypeResolved(get, nil, "", model).String(); got != `Illuminate\Database\Eloquent\Collection<int, App\Models\Category>` {
			t.Errorf("%s: Builder<Category>::get() = %q", framework, got)
		}
	}
}
