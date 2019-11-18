package graphql

import (
	"sync"
	"sync/atomic"
)

// NewImmediateGoroutineScheduler creates a new batch execution scheduler that
// executes all Units immediately in their own goroutine.
func NewImmediateGoroutineScheduler() *immediateGoroutineScheduler {
	return &immediateGoroutineScheduler{}
}

type immediateGoroutineScheduler struct {
	counter int64
	wgEarly sync.WaitGroup
	wgAll   sync.WaitGroup
}

func (r *immediateGoroutineScheduler) WaitEarly() {
	r.wgEarly.Wait()
}

func (r *immediateGoroutineScheduler) WaitAll() {
	r.wgAll.Wait()
}

func (r *immediateGoroutineScheduler) Counter() int64 {
	return r.counter
}

func (r *immediateGoroutineScheduler) Schedule(unit *WorkUnit) {
	atomic.AddInt64(&r.counter, 1)

	if len(unit.destinations) == 0 {
		panic("no destinations")
	}

	for _, d := range unit.destinations {
		if d.deferred != unit.destinations[0].deferred {
			panic("inconsistent defer")
		}
	}

	wg := &r.wgEarly
	if unit.destinations[0].deferred {
		wg = &r.wgAll
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ExecuteWorkUnit(r, unit)
	}()
}
