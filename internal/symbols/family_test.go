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
