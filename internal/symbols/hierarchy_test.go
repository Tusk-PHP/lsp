package symbols

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestDirectSupertypesAndSubtypes(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///h.php", `<?php
namespace App;
interface Named {}
interface Shape extends Named {}
trait Labels {}
abstract class Base implements Shape { use Labels; }
class Square extends Base implements \Countable { public function count(): int { return 0; } }
class Circle extends Base {}
`)
	sorted := func(s []string) []string { slices.Sort(s); return s }
	cases := []struct {
		got, want []string
	}{
		{sorted(idx.DirectSupertypes(`App\Base`)), []string{`App\Labels`, `App\Shape`}},
		{sorted(idx.DirectSupertypes(`App\Square`)), []string{`App\Base`, `Countable`}},
		{idx.DirectSupertypes(`App\Shape`), []string{`App\Named`}},
		{sorted(idx.DirectSubtypes(`App\Base`)), []string{`App\Circle`, `App\Square`}},
		{idx.DirectSubtypes(`App\Shape`), []string{`App\Base`}},
		{idx.DirectSubtypes(`App\Named`), []string{`App\Shape`}},
		{idx.DirectSubtypes(`App\Square`), nil},
	}
	for i, c := range cases {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("case %d: got %v, want %v", i, c.got, c.want)
		}
	}
}

func TestIsDeprecated(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///d.php", `<?php
/** @deprecated use New */
class Old {}
#[\Deprecated]
function gone() {}
class New {}
`)
	for fqn, want := range map[string]bool{"Old": true, "gone": true, "New": false} {
		if got := idx.Lookup(fqn).IsDeprecated(); got != want {
			t.Errorf("%s: got %v, want %v", fqn, got, want)
		}
	}
}

func BenchmarkDirectSubtypes(b *testing.B) {
	for _, classes := range []int{100, 10000} {
		idx := NewIndex()
		var src strings.Builder
		src.WriteString("<?php\ninterface Root {}\nclass Mid implements Root {}\n")
		for i := 0; i < classes; i++ {
			fmt.Fprintf(&src, "class C%d extends Mid {}\n", i)
		}
		idx.IndexFile("file:///bench.php", src.String())
		b.Run(fmt.Sprintf("classes=%d", classes), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if len(idx.DirectSubtypes("Root")) != 1 {
					b.Fatal("Root has one direct subtype")
				}
			}
		})
	}
}
