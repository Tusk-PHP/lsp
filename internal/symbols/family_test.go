package symbols

import (
	"slices"
	"testing"
)

func TestMemberFamilyIsWhatItOverridesAndImplementsAndEverythingBelow(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///shapes.php", `<?php
namespace App;
interface Measured { public function area(): float; }
interface Shape extends Measured {}
class Room implements Shape { public function area(): float { return 1.0; } private function secret() {} }
class Big extends Room { public function AREA(): float { return 2.0; } }
class Huge extends Big {}
class Circle implements Shape { public function area(): float { return 3.0; } }
class Plain { public function area(): float { return 4.0; } }
class Hidden extends Room { private function secret() {} }
`)
	names := func(syms []*Symbol) []string {
		var out []string
		for _, s := range syms {
			out = append(out, s.FQN)
		}
		slices.Sort(out)
		return out
	}
	want := []string{`App\Big::AREA`, `App\Circle::area`, `App\Measured::area`, `App\Room::area`}
	for _, from := range []string{`App\Room::area`, `App\Big::AREA`, `App\Measured::area`, `App\Circle::area`} {
		if got := names(idx.MemberFamily(idx.Lookup(from))); !slices.Equal(got, want) {
			t.Errorf("family of %s = %v, want %v", from, got, want)
		}
	}
	if got := names(idx.MemberFamily(idx.Lookup(`App\Plain::area`))); !slices.Equal(got, []string{`App\Plain::area`}) {
		t.Errorf("an unrelated class's method is its own family, got %v", got)
	}
	if got := names(idx.MemberFamily(idx.Lookup(`App\Room::secret`))); !slices.Equal(got, []string{`App\Room::secret`}) {
		t.Errorf("a private method is its own family, got %v", got)
	}
	if got := idx.MemberFamily(nil); got != nil {
		t.Errorf("no member, no family: %v", got)
	}
}

func TestMemberFamilyWalksADiamondAndACycleOnce(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///diamond.php", `<?php
namespace App;
interface Base { public function run(): void; }
interface Left extends Base {}
interface Right extends Base {}
class Job implements Left, Right { public function run(): void {} }
class Loop extends Knot { public function run(): void {} }
class Knot extends Loop { public function run(): void {} }
`)
	var got []string
	for _, s := range idx.MemberFamily(idx.Lookup(`App\Job::run`)) {
		got = append(got, s.FQN)
	}
	slices.Sort(got)
	if want := []string{`App\Base::run`, `App\Job::run`}; !slices.Equal(got, want) {
		t.Errorf("family across a diamond = %v, want %v, each once", got, want)
	}
	got = got[:0]
	for _, s := range idx.MemberFamily(idx.Lookup(`App\Loop::run`)) {
		got = append(got, s.FQN)
	}
	slices.Sort(got)
	if want := []string{`App\Knot::run`, `App\Loop::run`}; !slices.Equal(got, want) {
		t.Errorf("family across a cycle = %v, want %v", got, want)
	}
}

func TestMemberFamilyReachesTheClassesThatUseATrait(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///traits.php", `<?php
namespace App;
trait Greets { abstract public function greet(): void; }
trait Polite { use Greets; }
class Host { use Greets; public function greet(): void {} }
class Guest { use Polite; public function greet(): void {} }
class Child extends Host { public function greet(): void {} }
trait Waves { public function greet(): void {} }
class Picky { use Greets, Waves { Waves::greet insteadof Greets; } }
class PickyChild extends Picky { public function greet(): void {} }
class Renamer { use Greets { greet as hello; } public function greet(): void {} }
`)
	idx.IndexFile("file:///other.php", `<?php
namespace App;
class Leaving { use Greets; public function greet(): void {} }
`)
	family := func(from string) []string {
		var out []string
		for _, s := range idx.MemberFamily(idx.Lookup(from)) {
			out = append(out, s.FQN)
		}
		slices.Sort(out)
		return out
	}
	want := []string{`App\Child::greet`, `App\Greets::greet`, `App\Guest::greet`, `App\Host::greet`, `App\Leaving::greet`, `App\Renamer::greet`}
	if got := family(`App\Greets::greet`); !slices.Equal(got, want) {
		t.Errorf("family of a trait method = %v, want %v: its users, a trait's user, their subclasses, never what an insteadof rule replaces", got, want)
	}
	idx.IndexFile("file:///other.php", `<?php
namespace App;
class Leaving { public function greet(): void {} }
`)
	if got := family(`App\Greets::greet`); slices.Contains(got, `App\Leaving::greet`) {
		t.Errorf("a class that no longer uses the trait is still in its family: %v", got)
	}
	if got := family(`App\Waves::greet`); !slices.Equal(got, []string{`App\PickyChild::greet`, `App\Waves::greet`}) {
		t.Errorf("family of the trait method an insteadof rule keeps = %v", got)
	}
}

func TestMemberFamilyGivesAnAliasAsTheAlias(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///alias.php", `<?php
namespace App;
trait Talks { public function talk(): void {} }
class Speaker { public function speak(): void {} }
class Loud extends Speaker { use Talks { talk as speak; } }
`)
	var aliases []*Symbol
	for _, s := range idx.MemberFamily(idx.Lookup(`App\Speaker::speak`)) {
		if s.FQN == `App\Talks::talk` {
			t.Errorf("the method an alias overrides with brought the aliased trait method into the family")
		}
		if s.AliasOf != nil {
			aliases = append(aliases, s)
		}
	}
	if len(aliases) != 1 || aliases[0].FQN != `App\Loud::speak` || aliases[0].AliasOf.FQN != `App\Talks::talk` {
		t.Errorf("want Loud::speak as an alias of Talks::talk in the family, got %v", aliases)
	}
}

func TestTraitRuleMethodIsTheMethodOfTheTraitTheRuleNames(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///rules.php", `<?php
namespace App;
trait Talks { public function talk(): void {} }
trait Waves { public function talk(): void {} public function wave(): void {} }
class Both { use Talks, Waves { Waves::talk insteadof Talks; wave as hello; } }
`)
	for _, c := range []struct{ trait, method, want string }{
		{`App\Waves`, "talk", `App\Waves::talk`},
		{`App\Talks`, "talk", `App\Talks::talk`},
		{"", "wave", `App\Waves::wave`},
	} {
		if got := idx.TraitRuleMethod(`App\Both`, c.trait, c.method); got == nil || got.FQN != c.want {
			t.Errorf("TraitRuleMethod(Both, %q, %q) = %v, want %s", c.trait, c.method, got, c.want)
		}
	}
	if got := idx.TraitRuleMethod(`App\Both`, `App\Talks`, "wave"); got != nil {
		t.Errorf("Talks has no wave, got %s", got.FQN)
	}
}
