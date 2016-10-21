package thunder

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestConcurrencyLimiter tests that Acquire and ReleaseGoroutineToken bound
// parallelism.
func TestConcurrencyLimiter(t *testing.T) {
	const parallelism = 5

	ctx := WithConcurrencyLimiter(context.Background(), parallelism)

	var n int64 // track running calls

	var wg sync.WaitGroup
	var mu sync.Mutex
	max := 0 // track max running calls

	// run parallelism*2 calls concurrently
	for i := 0; i < parallelism*2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			AcquireGoroutineToken(ctx)
			defer ReleaseGoroutineToken(ctx)

			new := int(atomic.AddInt64(&n, 1)) // track execution
			time.Sleep(300 * time.Millisecond) // sleep a while so others can also run
			atomic.AddInt64(&n, -1)            // track execution

			mu.Lock()
			if new > max {
				max = new
			}
			mu.Unlock()
		}(i)
	}

	ReleaseGoroutineToken(ctx)
	wg.Wait()
	AcquireGoroutineToken(ctx)

	// expect exactly parallelism concurrent calls
	if max != parallelism {
		t.Error("expected bounded concurrent calls")
	}
}
