package graphql

import (
	"context"
	"sync"
	"sync/atomic"
)

// NewImmediateGoroutineScheduler creates a new batch execution scheduler that
// executes all Units immediately in their own goroutine.
func NewImmediateGoroutineScheduler() WorkScheduler {
	return &immediateGoroutineScheduler{}
}

type immediateGoroutineScheduler struct {
	wg    sync.WaitGroup
	count int64
}

func (q *immediateGoroutineScheduler) Run(resolver UnitResolver, initialUnits ...*WorkUnit) {
	r := &immediateGoroutineSchedulerRunner{}
	r.runEnqueue(resolver, initialUnits...)
	r.wg.Wait()
}

func (q *immediateGoroutineScheduler) RunAll(ctx context.Context, executionUnits []*ExecutionUnit) {
	q.runEnqueue(executionUnits)
	q.wg.Wait()
}

func (q *immediateGoroutineScheduler) runEnqueue(executionUnits []*ExecutionUnit) {
	atomic.AddInt64(&q.count, int64(len(executionUnits)))
	for _, unit := range executionUnits {
		q.wg.Add(1)
		go func(u *ExecutionUnit) {
			defer q.wg.Done()
			u.ExeucteFunction(u.Context)
		}(unit)
	}
}

type immediateGoroutineSchedulerRunner struct {
	wg sync.WaitGroup
}

func (r *immediateGoroutineSchedulerRunner) runEnqueue(resolver UnitResolver, units ...*WorkUnit) {
	for _, unit := range units {
		r.wg.Add(1)
		go func(u *WorkUnit) {
			defer r.wg.Done()
			units := resolver(u)
			r.runEnqueue(resolver, units...)
		}(unit)
	}
}
