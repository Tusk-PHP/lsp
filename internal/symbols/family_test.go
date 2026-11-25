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
