package resolve

import (
	"runtime"
	"sync"
)

// Parallel calls do for every i in [0, n) on a bounded pool of workers, one
// per CPU at most, each with its own Resolver from e, and returns when every
// call has returned. Each worker's Resolver keeps its own recursion guard and
// current file, while the Engine's caches are shared, so a request can
// resolve expressions in many files at once. do must not keep the Resolver
// after it returns, and must not write shared state without its own lock.
func (e *Engine) Parallel(n int, do func(r *Resolver, i int)) {
	workers := min(n, runtime.GOMAXPROCS(0))
	if workers <= 1 {
		r := e.Resolver()
		for i := range n {
			do(r, i)
		}
		return
	}
	next := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			r := e.Resolver()
			for i := range next {
				do(r, i)
			}
		}()
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()
}
