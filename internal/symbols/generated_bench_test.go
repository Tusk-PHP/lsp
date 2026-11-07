package symbols

import "testing"

// Lookups that miss are the only ones a class generator adds work to.
func benchmarkLookupMiss(b *testing.B, gen ClassGenerator) {
	idx := NewIndex()
	idx.IndexFile("file:///Product.php", "<?php\nnamespace App;\nclass Product {}\n")
	idx.SetClassGenerator(gen)
	idx.Lookup(`App\ProductFactory`) // built once, then a hit
	names := []string{`App\Missing`, `App\Product::missing`, `App\MissingFactory`, `App\ProductFactory`}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Lookup(names[i%len(names)])
	}
}

func BenchmarkLookupMissNoGenerator(b *testing.B) { benchmarkLookupMiss(b, nil) }
func BenchmarkLookupMissGenerator(b *testing.B)   { benchmarkLookupMiss(b, &factories{}) }
