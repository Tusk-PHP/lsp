package symbols

import "testing"

func BenchmarkClassMembersThroughTraits(b *testing.B) {
	idx := NewIndex()
	idx.IndexFile("file:///bench.php", `<?php
trait A { public function a1() {} public function a2() {} public function shared() {} public $p; }
trait B { public function b1() {} public function b2() {} public function shared() {} }
trait C { use A; public function c1() {} }
class Base { public function base() {} }
class Plain extends Base { use C, B; public function own() {} }
class Adapted extends Base { use C, B { A::shared insteadof B; B::shared as bShared; b1 as protected; } public function own() {} }
`)
	for _, class := range []string{"Plain", "Adapted"} {
		b.Run(class, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if len(idx.GetClassMembers(class)) == 0 {
					b.Fatal("no members")
				}
			}
		})
	}
}
