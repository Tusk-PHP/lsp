package resolve

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// Parallel calls do for every i in [0, n) on a bounded pool of workers, one
// per CPU at most, each with its own Resolver from e, and returns when every
// call has returned. Each worker's Resolver keeps its own recursion guard and
// current file, while the Engine's caches are shared, so a request can
// resolve expressions in many files at once. do must not keep the Resolver
// after it returns, and must not write shared state without its own lock.
//
// A panic in do stops the remaining calls and is raised again on the
// caller's goroutine, with the worker's stack, once every worker has stopped,
// so the request's own recovery sees it.
func (e *Engine) Parallel(n int, do func(r *Resolver, i int)) {
	workers := min(n, runtime.GOMAXPROCS(0))
	if workers <= 1 {
		r := e.Resolver()
		for i := range n {
			do(r, i)
		}
		return
	}
	var next atomic.Int64
	var failed atomic.Pointer[workerPanic]
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					failed.CompareAndSwap(nil, &workerPanic{value: v, stack: debug.Stack()})
				}
			}()
			r := e.Resolver()
			for failed.Load() == nil {
				i := int(next.Add(1) - 1)
				if i >= n {
					return
				}
				do(r, i)
			}
		}()
	}
	wg.Wait()
	if p := failed.Load(); p != nil {
		panic(p)
	}
}

// workerPanic is a panic raised by a call of Parallel on one of its workers,
// with that worker's stack.
type workerPanic struct {
	value any
	stack []byte
}

func (p *workerPanic) Error() string {
	return fmt.Sprintf("panic in a resolve.Parallel worker: %v\n%s", p.value, p.stack)
}
