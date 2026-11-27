package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func typeOf(in TypedInputs, name string) string {
	for _, v := range in.Vars {
		if v.Name == name {
			return v.Type
		}
	}
	return ""
}

func TestViewVariablesPassedFromPHPAreTypedWhereTheyArePassed(t *testing.T) {
	controller := "file:///app/app/Http/Controllers/UserController.php"
	idx := symbols.NewIndex()
	idx.IndexFile(controller, controllerSrc)
	data := NewViewData("/app")
	data.IndexFile(controller, controllerSrc)
	types := &ViewTypes{
		Index:    idx,
		Data:     data,
		Resolver: resolve.NewEngine(idx).Resolver,
	}

	in := types.For("file:///app/resources/views/users/show.blade.php")
	if got := typeOf(in, "user"); got != `App\Models\User` {
		t.Errorf("$user is typed in the controller that passes it: got %q", got)
	}
	if got := typeOf(in, "count"); got != "int" {
		t.Errorf("a literal is typed without resolving: got %q", got)
	}
	if got := typeOf(types.For("file:///app/resources/views/users/index.blade.php"), "filter"); got != "string" {
		t.Errorf("a variable passed with compact() is typed where it is passed: got %q", got)
	}
}

func TestComponentPropertiesAreTypedAgainstTheirFilesImports(t *testing.T) {
	component := "file:///app/app/View/Components/Receipt.php"
	src := `<?php
namespace App\View\Components;

use App\Models\Order;
use Illuminate\View\Component;

class Receipt extends Component
{
    public Order $order;

    public function render()
    {
        return view('components.receipt');
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/vendor/Component.php", "<?php\nnamespace Illuminate\\View;\nabstract class Component {}\n")
	idx.IndexFile(component, src)
	data := NewViewData("/app")
	data.IndexFile(component, src)
	types := &ViewTypes{Index: idx, Data: data}

	if got := typeOf(types.For("file:///app/resources/views/components/receipt.blade.php"), "order"); got != `App\Models\Order` {
		t.Errorf("the component's $order is the imported Order: got %q", got)
	}
}

func TestViewVariablesPassedWithCompactAreTypedAsTheVariable(t *testing.T) {
	controller := "file:///app/app/Http/Controllers/ProfileController.php"
	src := `<?php
namespace App\Http\Controllers;

use App\Models\User;

class ProfileController
{
    public function show(User $user)
    {
        return view('profile.show', compact('user'));
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile(controller, src)
	data := NewViewData("/app")
	data.IndexFile(controller, src)
	types := &ViewTypes{Index: idx, Data: data, Resolver: resolve.NewEngine(idx).Resolver}
	if got := typeOf(types.For("file:///app/resources/views/profile/show.blade.php"), "user"); got != `App\Models\User` {
		t.Errorf("compact('user') passes the $user parameter: got %q", got)
	}
	st := &typingState{visiting: map[string]bool{}, files: map[string]siteFile{}}
	if got := types.siteType(ViewVar{URI: controller, Expr: "$user", ExprLine: 99}, st); got != "" {
		t.Errorf("a site past the end of its file: got %q", got)
	}
	if got := types.siteType(ViewVar{URI: controller, Expr: "App\\Http", ExprLine: 1, ExprCol: len("namespace ")}, st); got != "" {
		t.Errorf("a site that is no expression: got %q", got)
	}
}
