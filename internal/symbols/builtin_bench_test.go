package symbols

import (
	"runtime"
	"testing"
)

func BenchmarkRegisterBuiltins(b *testing.B) {
	b.ReportAllocs()
	var idx *Index
	for i := 0; i < b.N; i++ {
		idx = NewIndex()
		idx.RegisterBuiltins()
	}
	b.StopTimer()
	var before, after runtime.MemStats
	idx = nil
	runtime.GC()
	runtime.ReadMemStats(&before)
	idx = NewIndex()
	idx.RegisterBuiltins()
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(idx)
	b.ReportMetric(float64(after.HeapAlloc)-float64(before.HeapAlloc), "retained-B")
}
