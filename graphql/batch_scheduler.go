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

func (q *immediateGoroutineScheduler) Run(resolver UnitResolver, initialUnits ...*WorkUnit) {
	q.runEnqueue(resolver, initialUnits...)

	q.wg.Wait()
}

func (q *immediateGoroutineScheduler) runEnqueue(resolver UnitResolver, units ...*WorkUnit) {
	for _, unit := range units {
		q.wg.Add(1)
		go func(u *WorkUnit) {
			defer q.wg.Done()
			units := resolver(u)
			q.runEnqueue(resolver, units...)
		}(unit)
	}
}
