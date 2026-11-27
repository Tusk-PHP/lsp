package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
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
		Index: idx,
		Data:  data,
		Resolve: func(expr, _ string, file *parser.FileNode, _, _ int) string {
			if expr == "$user" && file != nil && file.Namespace == `App\Http\Controllers` {
				return `App\Models\User`
			}
			return ""
		},
	}

	in := types.For("file:///app/resources/views/users/show.blade.php")
	if got := typeOf(in, "user"); got != `App\Models\User` {
		t.Errorf("$user is typed in the controller that passes it: got %q", got)
	}
	if got := typeOf(in, "count"); got != "int" {
		t.Errorf("a literal is typed without resolving: got %q", got)
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
