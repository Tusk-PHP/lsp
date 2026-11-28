package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestTemplateFallbacksTypeClassesWithoutTemplates(t *testing.T) {
	src := `<?php
namespace App;
class User {}
class Bag {
    public function first() {}
    public function where() {}
    public function count(): int { return 0; }
    public function nothing() {}
    public function done(): void {}
    /** @return key-of<Missing> */
    public function unknownKey() {}
}
class SubBag extends Bag {}
/** @template T */
class Typed {
    public function first() {}
}
class Loose {
    /**
     * @param Bag<User> $bag
     * @param SubBag<User> $sub
     * @param Typed<User> $typed
     */
    public function run(Bag $bag, SubBag $sub, Typed $typed): void {
        echo $bag; // here
    }
}
`
	r, ctx := fixtureAt(t, src, "here")
	r.Index.SetTemplateFallbacks(map[string]symbols.TemplateFallback{
		`App\Bag`:   {Params: []string{"TValue"}, Methods: map[string]string{"first": "?TValue", "where": "static"}},
		`App\Typed`: {Params: []string{"T"}, Methods: map[string]string{"first": "?T"}},
	})
	for expr, want := range map[string]string{
		"$bag->first()":      `?App\User`,
		"$bag->where()":      `App\Bag<App\User>`,
		"$sub->where()":      `App\SubBag<App\User>`,
		"$bag->count()":      "int",
		"$bag->nothing()":    "",
		"$bag->done()":       "",
		"$bag->unknownKey()": "key-of<Missing>",
		"$typed->first()":    "",
	} {
		if got := r.exprType(expr, ctx).FullString(); got != want {
			t.Errorf("ExpressionType(%q) = %q, want %q", expr, got, want)
		}
	}
}

func TestTemplateFallbacksNeedAnIndex(t *testing.T) {
	m := &symbols.Symbol{Kind: symbols.KindMethod, Name: "first", ParentFQN: `App\Bag`}
	if got := NewResolver(nil).MemberTypeForCall(m, nil, `App\Bag`, []ResolvedType{{FQN: "int"}}, nil); !got.IsEmpty() {
		t.Errorf("MemberTypeForCall without an index = %q, want nothing", got.FullString())
	}
}
