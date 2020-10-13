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

func (q *immediateGoroutineScheduler) Run(resolver UnitResolver, initialUnits ...*WorkUnit) string {
	r := &immediateGoroutineSchedulerRunner{}
	r.runEnqueue(resolver, initialUnits...)

	r.wg.Wait()
	fmt.Println("GJVHBKJNLKM:L<", r.errors)
	return r.errors
	// fmt.Println("EEEEE", e)
}

type immediateGoroutineSchedulerRunner struct {
	wg     sync.WaitGroup
	errors string
}

func (r *immediateGoroutineSchedulerRunner) runEnqueue(resolver UnitResolver, units ...*WorkUnit) {
	// errors2 := ""
	for _, unit := range units {
		r.wg.Add(1)
		go func(u *WorkUnit) {
			defer r.wg.Done()
			units, errors := resolver(u)
			fmt.Println("YEEEEEEET", errors)
			r.errors = errors
			// fmt.Println("GYIUHOIJL", errors)
			// errors2 = errors
			// fmt.Println("GYIUHOIJL", errors2)
			r.runEnqueue(resolver, units...)
		}(unit)
	}
	// r.wg.Wait()
	// fmt.Println("ERROS2", errors2)
	// return errors2
}
