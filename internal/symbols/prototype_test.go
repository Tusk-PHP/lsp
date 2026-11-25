package symbols

import (
	"fmt"
	"strings"
	"testing"
)

func prototypeIndex(t *testing.T) *Index {
	t.Helper()
	idx := NewIndex()
	idx.RegisterBuiltins()
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
	idx.IndexFile("file:///Model.php", `<?php
namespace App;
trait HasAttributes { protected function casts(): array { return []; } }
abstract class Model implements \ArrayAccess, \JsonSerializable {
    use HasAttributes;
    abstract public function offsetGet(mixed $offset): mixed;
}
class User extends Model {
    protected function casts(): array { return ['a' => 'b']; }
    public function offsetGet(mixed $offset): mixed { return null; }
    public function offsetExists(mixed $offset): bool { return false; }
    public function jsonSerialize(): mixed { return []; }
}
trait Measures { public function area(): float { return 2.0; } }
class ViaTrait implements Shape { use Measures; public function name(): string { return 'v'; } }
interface HasLabel { public function label(): string; const DEFAULT = 'none'; }
enum Suit: string implements HasLabel {
    case Hearts = 'h';
    const DEFAULT = 'hearts';
    public function label(): string { return 'suit'; }
}
`)
	idx.IndexFile("file:///Rules.php", `<?php
namespace App;
class Secretive { private function secret(): void {} public function __construct() {} }
class Teller extends Secretive { public function secret(): void {} public function __construct() {} }
abstract class Built { abstract public function __construct(int $x); }
class Builder extends Built { public function __construct(int $x) {} }
interface Makes { public function __construct(); }
class Maker implements Makes { public function __construct() {} }
trait Hidden { abstract private function hidden(): void; }
class UsesHidden { use Hidden; private function hidden(): void {} }
class BelowHidden extends UsesHidden { private function hidden(): void {} }
`)
	return idx
}

func TestPrototypeIsTheTopmostDeclaration(t *testing.T) {
	idx := prototypeIndex(t)
	cases := []struct{ receiver, member, want string }{
		{"", `App\Square::area`, `App\Shape::area`},
		{"", `App\Square::name`, `App\Named::name`},
		{"", `App\Base::NAME`, `App\Named::name`},
		{"", `App\Square::draw`, `App\Base::draw`},
		{"", `App\Square::$label`, `App\Base::$label`},
		{"", `App\Square::PREFIX`, `App\Named::PREFIX`},
		{"", `App\User::casts`, `App\HasAttributes::casts`},
		{`App\ViaTrait`, `App\Measures::area`, `App\Shape::area`},
		{"", `App\User::offsetGet`, `App\Model::offsetGet`},
		{"", `App\Suit::label`, `App\HasLabel::label`},
		{"", `App\Suit::DEFAULT`, `App\HasLabel::DEFAULT`},
		{"", `App\Builder::__construct`, `App\Built::__construct`},
		{"", `App\Maker::__construct`, `App\Makes::__construct`},
		{"", `App\UsesHidden::hidden`, `App\Hidden::hidden`},
	}
	for _, c := range cases {
		got := idx.Prototype(c.receiver, idx.Lookup(c.member))
		if got == nil || got.FQN != c.want {
			t.Errorf("%s through %q: got %v, want %s", c.member, c.receiver, got, c.want)
		}
	}
}

func TestPrototypeOfAMemberNothingAboveDeclares(t *testing.T) {
	idx := prototypeIndex(t)
	cases := []struct{ receiver, member string }{
		{"", `App\Square::side`},
		{"", `App\Named::name`},
		{"", `App\Square`},
		{"", `App\Base::missing`},
		{"", `App\Orphan::run`},
		{`App\Square`, `App\Square::side`},
		{`App\Square`, `App\Base::draw`},
		{`App\ViaTrait`, `App\Measures::name`},
		{`App\Orphan`, `App\Measures::area`},
		{"", `App\User::offsetExists`},
		{"", `App\User::jsonSerialize`},
		{"", `App\Teller::secret`},
		{"", `App\Teller::__construct`},
		{"", `App\BelowHidden::hidden`},
	}
	for _, c := range cases {
		if got := idx.Prototype(c.receiver, idx.Lookup(c.member)); got != nil {
			t.Errorf("%s through %q: got %s, want none", c.member, c.receiver, got.FQN)
		}
	}
	if idx.Prototype("", nil) != nil {
		t.Error("no symbol, no prototype")
	}
	if got := idx.Prototype("", idx.Lookup(`App\Loop::run`)); got == nil || got.FQN != `App\Loop2::run` {
		t.Errorf("a cycle in the hierarchy ends: got %v", got)
	}
}

func TestPrototypeFollowsTraitInsteadofAndAs(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///Talk.php", `<?php
namespace App;
interface Speaks { public function bark(): void; }
trait A { public function talk(): void {} public function shout(): void {} }
trait B { public function talk(): void {} public function whisper(): void {} }
trait Nested { use B; }
trait Polite { use Nested { whisper as hush; } }
class Talker implements Speaks { use B, A { A::talk insteadof B; B::talk as bTalk; A::shout as bark; } }
class NestedTalker { use Nested, A { A::talk insteadof Nested; } }
class NestedAlias { use Nested { Nested::whisper as murmur; } }
class Hider { use B { whisper as private secretly; } }
class TalkerChild extends Talker { public function talk(): void {} public function bTalk(): void {} }
class NestedAliasChild extends NestedAlias { public function murmur(): void {} }
class PoliteHost { use Polite; }
class PoliteChild extends PoliteHost { public function hush(): void {} }
class HiderChild extends Hider { public function secretly(): void {} }
`)
	declared := []struct{ receiver, member, want string }{
		{"", `App\TalkerChild::talk`, `App\A::talk`},
		{"", `App\TalkerChild::bTalk`, `App\B::talk`},
		{"", `App\NestedAliasChild::murmur`, `App\B::whisper`},
		{"", `App\PoliteChild::hush`, `App\B::whisper`},
	}
	for _, c := range declared {
		if got := idx.Prototype(c.receiver, idx.Lookup(c.member)); got == nil || got.FQN != c.want {
			t.Errorf("%s: got %v, want %s", c.member, got, c.want)
		}
	}
	if bark := idx.ClassMembersNamed(`App\Talker`, "bark"); len(bark) != 1 {
		t.Errorf("Talker has the alias bark once: %v", bark)
	} else if got := idx.Prototype(`App\Talker`, bark[0]); got == nil || got.FQN != `App\Speaks::bark` {
		t.Errorf("an alias implementing an interface method: got %v, want App\\Speaks::bark", got)
	}
	undeclared := []struct{ receiver, member string }{
		{`App\Talker`, `App\A::talk`},
		{`App\NestedTalker`, `App\A::talk`},
		{"", `App\HiderChild::secretly`},
	}
	for _, c := range undeclared {
		if got := idx.Prototype(c.receiver, idx.Lookup(c.member)); got != nil {
			t.Errorf("%s through %q: got %s, want none", c.member, c.receiver, got.FQN)
		}
	}
}

func BenchmarkPrototype(b *testing.B) {
	for _, classes := range []int{100, 10000} {
		idx := NewIndex()
		var src strings.Builder
		src.WriteString("<?php\ninterface Root { public function run(): void; }\ntrait Runs { public function run(): void {} }\n")
		for i := 0; i < classes; i++ {
			fmt.Fprintf(&src, "class C%d implements Root { use Runs; }\n", i)
		}
		idx.IndexFile("file:///bench.php", src.String())
		member := idx.Lookup("Runs::run")
		b.Run(fmt.Sprintf("classes=%d", classes), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if idx.Prototype("C0", member) == nil {
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
