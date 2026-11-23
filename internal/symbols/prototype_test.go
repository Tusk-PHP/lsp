package symbols

import (
	"fmt"
	"strings"
	"testing"
)

func prototypeIndex(t *testing.T) *Index {
	t.Helper()
	idx := NewIndex()
	idx.IndexFile("file:///Shape.php", `<?php
namespace App;
interface Named { public function name(): string; const PREFIX = 'x'; }
interface Shape extends Named { public function area(): float; }
abstract class Base implements Shape {
    abstract public function area(): float;
    public function NAME(): string { return 'base'; }
    public function draw(): void {}
    protected string $label = '';
}
class Square extends Base {
    public function area(): float { return 1.0; }
    public function name(): string { return 'square'; }
    public function draw(): void {}
    public function side(): float { return 1.0; }
    protected string $label = 'square';
    const PREFIX = 'sq';
}
class Orphan extends Missing { public function run(): void {} }
class Loop extends Loop2 { public function run(): void {} }
class Loop2 extends Loop { public function run(): void {} }
`)
	return idx
}

func TestPrototypeIsTheTopmostDeclaration(t *testing.T) {
	idx := prototypeIndex(t)
	cases := map[string]string{
		`App\Square::area`:   `App\Shape::area`,
		`App\Square::name`:   `App\Named::name`,
		`App\Base::NAME`:     `App\Named::name`,
		`App\Square::draw`:   `App\Base::draw`,
		`App\Square::$label`: `App\Base::$label`,
		`App\Square::PREFIX`: `App\Named::PREFIX`,
	}
	for member, want := range cases {
		got := idx.Prototype(idx.Lookup(member))
		if got == nil || got.FQN != want {
			t.Errorf("%s: got %v, want %s", member, got, want)
		}
	}
}

func TestPrototypeOfAMemberNothingAboveDeclares(t *testing.T) {
	idx := prototypeIndex(t)
	for _, fqn := range []string{`App\Square::side`, `App\Named::name`, `App\Square`, `App\Base::missing`, `App\Orphan::run`} {
		if got := idx.Prototype(idx.Lookup(fqn)); got != nil {
			t.Errorf("%s: got %s, want none", fqn, got.FQN)
		}
	}
	if idx.Prototype(nil) != nil {
		t.Error("no symbol, no prototype")
	}
	if got := idx.Prototype(idx.Lookup(`App\Loop::run`)); got == nil || got.FQN != `App\Loop2::run` {
		t.Errorf("a cycle in the hierarchy ends: got %v", got)
	}
}

func BenchmarkPrototype(b *testing.B) {
	for _, classes := range []int{100, 10000} {
		idx := NewIndex()
		var src strings.Builder
		src.WriteString("<?php\ninterface Root { public function run(): void; }\n")
		for i := 0; i < classes; i++ {
			fmt.Fprintf(&src, "class C%d implements Root { public function run(): void {} }\n", i)
		}
		idx.IndexFile("file:///bench.php", src.String())
		member := idx.Lookup("C0::run")
		b.Run(fmt.Sprintf("classes=%d", classes), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if idx.Prototype(member) == nil {
					b.Fatal("no prototype")
				}
			}
		})
	}
}

func TestFQNPrefixSearchListsOnlyTopLevelSymbols(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///shop.php", `<?php
namespace Shop;
class Cart { const MAX = 1; public $items; public function add() {} }
enum Size { case Small; }
function checkout() {}
`)
	syms, _ := idx.SearchByFQNPrefix(`Shop\`)
	var got []string
	for _, s := range syms {
		if s.Kind.IsMember() {
			t.Errorf("member %s is listed", s.FQN)
		}
		got = append(got, s.FQN)
	}
	if len(got) != 3 {
		t.Errorf("want Cart, Size and checkout, got %v", got)
	}
}
