package graphql

import (
	"fmt"
	"sync"
)

// NewImmediateGoroutineScheduler creates a new batch execution scheduler that
// executes all Units immediately in their own goroutine.
func NewImmediateGoroutineScheduler() WorkScheduler {
	return &immediateGoroutineScheduler{}
}

type immediateGoroutineScheduler struct{}

func (q *immediateGoroutineScheduler) Run(resolver UnitResolver, initialUnits ...*WorkUnit) []error {
	r := &immediateGoroutineSchedulerRunner{}
	r.runEnqueue(resolver, initialUnits...)

	r.wg.Wait()
	return r.errors
}

type immediateGoroutineSchedulerRunner struct {
	wg     sync.WaitGroup
	errors []error
}

func (r *immediateGoroutineSchedulerRunner) runEnqueue(resolver UnitResolver, units ...*WorkUnit) {
	for _, unit := range units {
		r.wg.Add(1)
		go func(u *WorkUnit) {
			defer r.wg.Done()
			units, errors := resolver(u)
			fmt.Println("YEEEEEEET", errors)
			r.errors = append(r.errors, errors...)
			r.runEnqueue(resolver, units...)
		}(unit)
	}
}
