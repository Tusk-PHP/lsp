package completion

import (
	"sort"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// TestNameRankerOrder checks the order names rank in, editing a file in
// App\Http\Controllers: imported first, then the same namespace, its
// sub-namespaces, siblings under each parent going up, the project's other
// roots, vendor packages and PHP's builtins last.
func TestNameRankerOrder(t *testing.T) {
	rk := newNameRanker(`App\Http\Controllers`, []parser.UseNode{{FullName: `Illuminate\Http\Request`}})
	project := func(fqn string) *symbols.Symbol {
		return &symbols.Symbol{Name: fqn[strings.LastIndexByte(fqn, '\\')+1:], FQN: fqn, Source: symbols.SourceProject}
	}
	want := []*symbols.Symbol{
		{Name: "Request", FQN: `Illuminate\Http\Request`, Source: symbols.SourceVendor},
		project(`App\Http\Controllers\UserController`),
		project(`App\Http\Controllers\Admin\PostController`),
		project(`App\Http\Controllers\Admin\Reports\SalesController`),
		project(`App\Http\Requests\StoreUserRequest`),
		project(`App\Http\Middleware\Deep\Nested\Thing`),
		project(`App\Models\User`),
		project(`Database\Factories\UserFactory`),
		{Name: "Collection", FQN: `Illuminate\Support\Collection`, Source: symbols.SourceVendor},
		{Name: "DateTime", FQN: "DateTime", Source: symbols.SourceBuiltin},
	}
	keys := make([]string, len(want))
	for i, sym := range want {
		keys[i] = rk.key(sym, sym.Name)
	}
	if !sort.StringsAreSorted(keys) {
		for i, sym := range want {
			t.Logf("%-55s %s", sym.FQN, keys[i])
		}
		t.Error("names are not ranked closest first")
	}
	if kw := keywordKey("return"); kw < rk.key(want[8], want[8].Name) {
		t.Errorf("keywords should rank after vendor names, got %q", kw)
	}
}

// TestCompleteVariablesClosestFirst checks `$` completion in a method: its
// variables latest assigned first, then its parameters in order, then $this,
// then the superglobals, and nothing assigned below the cursor.
func TestCompleteVariablesClosestFirst(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")
	source := `<?php
namespace App;

class Report
{
    public function build(array $rows, int $limit)
    {
        $total = 0;
        foreach ($rows as $row) {
            $total += $row;
        }
        $myVar = $total;
        $
        $later = 1;
    }
}
`
	items := p.GetCompletions("file:///r.php", source, protocol.Position{Line: 12, Character: 9})
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	want := []string{"$myVar", "$total", "$row", "$rows", "$limit", "$this"}
	if len(labels) < len(want) || strings.Join(labels[:len(want)], " ") != strings.Join(want, " ") {
		t.Fatalf("got %v, want it to start with %v", labels, want)
	}
	for _, l := range labels {
		if l == "$later" {
			t.Errorf("$later is assigned below the cursor and shouldn't be offered")
		}
	}
	if last := labels[len(labels)-1]; !strings.HasPrefix(last, "$_") && last != "$GLOBALS" {
		t.Errorf("superglobals should come last, got %v", labels)
	}

	items = p.GetCompletions("file:///r.php", strings.Replace(source, "        $\n", "        $li\n", 1), protocol.Position{Line: 12, Character: 11})
	if len(items) != 1 || items[0].Label != "$limit" {
		t.Errorf("typing $li: got %v", items)
	}
}

// TestMemberCompletionOwnBeforeInherited checks that after `->` the class's own
// members come before its traits' and its parent's, and an overridden method
// is listed once.
func TestMemberCompletionOwnBeforeInherited(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///m.php", `<?php
class Base { public function save() {} public function aBase() {} }
trait Greets { public function aTrait() {} }
class Child extends Base { use Greets; public function save() {} public function zOwn() {} }
`)
	p := NewProvider(idx, nil, "")
	source := "<?php\n$c = new Child();\n$c->\n"
	items := p.GetCompletions("file:///u.php", source, protocol.Position{Line: 2, Character: 4})
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	if got := strings.Join(labels, " "); got != "save zOwn aTrait aBase" {
		t.Errorf("got %q, want own, then trait, then parent, with save() once", got)
	}
}
