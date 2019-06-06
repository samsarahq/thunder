package reactive

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Expect is a utility for verifying that goroutines make progress.
type Expect struct {
	ch chan struct{}
}

// NewExpect creates a new Expect.
func NewExpect() *Expect {
	return &Expect{
		ch: make(chan struct{}),
	}
}

// Trigger lets a goroutine notify it has made progress.
func (e *Expect) Trigger() {
	close(e.ch)
}

// Expect lets a tester wait for a goroutine to make progress. Expect is fast
// in the common case but might block for 2 seconds if progress is a little
// slower due to scheduling.
func (e *Expect) Expect(t *testing.T, s string) {
	select {
	case <-e.ch:
		return
	case <-time.After(2 * time.Second):
		t.Error(s)
	}
}

// TestRerun tests that a computation is rerun after it is invalidated.
func TestRerun(t *testing.T) {
	released := NewExpect()
	dep := NewResource()
	dep.Cleanup(func() {
		released.Trigger()
	})

	run := NewExpect()

	runner := NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		AddDependency(ctx, dep, nil)
		run.Trigger()
		return nil, nil
	}, 0, false)

	for i := 0; i < 10; i++ {
		run.Expect(t, "expected (re-)run")
		run = NewExpect()
		dep.Strobe()
	}

	runner.Stop()
	released.Expect(t, "expected release")
}

// TestCache tests that a cached computation is not rerun when still valid.
func TestCache(t *testing.T) {
	dep := NewResource()

	run := NewExpect()
	innerRun := NewExpect()

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		AddDependency(ctx, dep, nil)

		Cache(ctx, 0, func(ctx context.Context) (interface{}, error) {
			innerRun.Trigger()
			return nil, nil
		})

		run.Trigger()
		return nil, nil
	}, 0, false)

	run.Expect(t, "expected run")
	innerRun.Expect(t, "expected inner run")

	run = NewExpect()
	dep.Strobe()

	run.Expect(t, "expected rerun")
	// inner run is expected cache; if it runs, it will panic in calling Trigger
}

// TestCachePurge tests that a cached computation is rerun when the cache is purged
func TestCachePurge(t *testing.T) {
	dep := NewResource()

	run := NewExpect()

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		AddDependency(ctx, dep, nil)

		Cache(ctx, 0, func(ctx context.Context) (interface{}, error) {
			run.Trigger()
			return nil, nil
		})

		PurgeCache(ctx)
		return nil, nil
	}, 0, false)

	run.Expect(t, "expected run")

	run = NewExpect()
	dep.Strobe()

	run.Expect(t, "expected rerun")
}

// TestRerunCache tests that a cached computation is rerun after it is invalidated.
func TestRerunCache(t *testing.T) {
	dep := NewResource()

	run := NewExpect()

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		Cache(ctx, 0, func(ctx context.Context) (interface{}, error) {
			AddDependency(ctx, dep, nil)
			run.Trigger()
			return nil, nil
		})
		return nil, nil
	}, 0, false)

	run.Expect(t, "expected run")

	run = NewExpect()
	dep.Strobe()

	run.Expect(t, "expected rerun")
}

// TestStop tests that a runner stops recomputating after Stop is called.
func TestStop(t *testing.T) {
	dep := NewResource()

	run := NewExpect()

	runner := NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		AddDependency(ctx, dep, nil)
		run.Trigger()
		return nil, nil
	}, 0, false)

	run.Expect(t, "expected run")

	runner.Stop()
	dep.Invalidate()

	// run is supposed to stop; if it runs, it will panic in calling Trigger
}

func TestError(t *testing.T) {
	dep := NewResource()

	run := NewExpect()

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		AddDependency(ctx, dep, nil)
		run.Trigger()
		return nil, errors.New("error")
	}, 0, false)

	run.Expect(t, "expected run")

	dep.Invalidate()

	// run is supposed to stop; if it runs, it will panic in calling Trigger
}

// TestErrorRetry verifies that the rerunner keeps the cache around
// and does not stop the rerunner if the retry sentinel is passed down.
func TestErrorRetry(t *testing.T) {
	dep := NewResource()
	run := NewExpect()

	innerRuns := 0
	shouldSentinel := false

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		AddDependency(ctx, dep, nil)
		Cache(ctx, "", func(ctx context.Context) (interface{}, error) {
			innerRuns = innerRuns + 1
			return nil, nil
		})

		if shouldSentinel {
			oldRun := run
			run = NewExpect()
			oldRun.Trigger()
			return nil, RetrySentinelError
		} else {
			run.Trigger()
		}
		return nil, nil
	}, 0, false)

	run.Expect(t, "expected run")
	if innerRuns != 1 {
		t.Errorf("expected 1 run, but got %d", innerRuns)
	}

	run = NewExpect()
	dep.Strobe()

	run.Expect(t, "expected rerun")
	if innerRuns != 1 {
		t.Errorf("expected 1 run, but got %d", innerRuns)
	}

	run = NewExpect()
	shouldSentinel = true
	dep.Strobe()

	run.Expect(t, "expected rerun with sentinel")
	if innerRuns != 1 {
		t.Errorf("expected 1 run, but got %d", innerRuns)
	}

	// The runner has not stopped because of our retry.

	shouldSentinel = false
	run.Expect(t, "expected rerun after sentinel")
	if innerRuns != 2 {
		t.Errorf("expected 2 runs (first run, then retry run), but got %d", innerRuns)
	}
}

// TestErrorRetryDelay verifies that retries are delayed exponentially.
func TestErrorRetryDelay(t *testing.T) {
	run := NewExpect()

	var lastRunTime time.Time
	var lastDelta time.Duration

	runner := NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		if !lastRunTime.IsZero() {
			lastDelta = time.Now().Sub(lastRunTime)
		}
		lastRunTime = time.Now()

		oldRun := run
		run = NewExpect()
		oldRun.Trigger()

		return nil, RetrySentinelError
	}, 100*time.Millisecond, false)

	run.Expect(t, "expected first run")

	for _, delay := range []time.Duration{time.Millisecond * 200, time.Millisecond * 400, time.Millisecond * 800} {
		run.Expect(t, "expected delayed run")
		if lastDelta < delay {
			t.Errorf("expected delay of at least %d but got %d", delay, lastDelta)
		}
	}

	runner.Stop()
}

// TestCacheLock tests that concurrent calls to Cache with the same key result
// in only one execution.
func TestCacheLock(t *testing.T) {
	run := NewExpect()

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		var n int64 // track executions

		// run 10 calls concurrently
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				Cache(ctx, "", func(ctx context.Context) (interface{}, error) {
					atomic.AddInt64(&n, 1)             // track execution
					time.Sleep(200 * time.Millisecond) // sleep a while so others can also run
					return nil, nil
				})
			}()
		}
		wg.Wait()

		if n != 1 {
			t.Error("expected at most 1 call")
		}

		run.Trigger()
		return nil, nil
	}, 0, false)

	run.Expect(t, "expected run")
}

// TestCacheParallel tests that concurrent calls to Cache with different keys
// result in parallel executions.
func TestCacheParallel(t *testing.T) {
	run := NewExpect()

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		var n int64 // track running calls

		var mu sync.Mutex
		max := 0 // track max running calls

		// run 10 calls concurrently
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				Cache(ctx, i, func(ctx context.Context) (interface{}, error) {
					new := int(atomic.AddInt64(&n, 1)) // track execution
					time.Sleep(200 * time.Millisecond) // sleep a while so others can also run
					atomic.AddInt64(&n, -1)            // track execution

					mu.Lock()
					if new > max {
						max = new
					}
					mu.Unlock()
					return nil, nil
				})
			}(i)
		}
		wg.Wait()

		if max <= 1 {
			t.Error("expected concurrent calls")
		}

		run.Trigger()
		return nil, nil
	}, 0, false)

	run.Expect(t, "expected run")
}

// TestMinRerunInterval tests that a runner debounces reruns
func TestMinRerunInterval(t *testing.T) {
	run := NewExpect()

	r := NewResource()
	var ran time.Time

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		AddDependency(ctx, r, nil)
		run.Trigger()

		if ran.IsZero() {
			ran = time.Now()
		} else {
			delta := time.Now().Sub(ran)
			if delta < 800*time.Millisecond {
				t.Error("expected at least 800ms delay")
			}
		}

		return nil, nil
	}, 1*time.Second, false)

	run.Expect(t, "expected run")

	run = NewExpect()
	r.Invalidate()
	run.Expect(t, "expected rerun")
}

// TestRerunImmediately tests that RerunImmediately bypasses the
// rerun delay.
func TestRerunImmediately(t *testing.T) {
	run := NewExpect()

	r := NewResource()
	var ran time.Time

	runner := NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		AddDependency(ctx, r, nil)
		run.Trigger()

		if ran.IsZero() {
			ran = time.Now()
		} else {
			delta := time.Now().Sub(ran)
			if delta > 1*time.Second {
				t.Error("expected imediate rerun")
			}
		}

		return nil, nil
	}, 30*time.Second, false)

	run.Expect(t, "expected run")

	run = NewExpect()
	runner.RerunImmediately()
	r.Invalidate()
	run.Expect(t, "expected rerun")
}
