package graphql

import (
	"sync"
)

// NewImmediateGoroutineScheduler creates a new batch execution scheduler that
// executes all Units immediately in their own goroutine.
func NewImmediateGoroutineScheduler() WorkScheduler {
	return &immediateGoroutineScheduler{}
}

type immediateGoroutineScheduler struct {
	wg sync.WaitGroup
}

func (r *immediateGoroutineScheduler) Run() {
	r.wg.Wait()
}

func (r *immediateGoroutineScheduler) Schedule(unit *WorkUnit) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ExecuteWorkUnit(r, unit)
	}()
}
