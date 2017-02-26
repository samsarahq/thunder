package reactive

import (
	"context"
	"errors"
	"strings"
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
		AddDependency(ctx, dep)
		run.Trigger()
		return nil, nil
	}, 0)

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
		AddDependency(ctx, dep)

		Cache(ctx, 0, func(ctx context.Context) (interface{}, error) {
			innerRun.Trigger()
			return nil, nil
		})

		run.Trigger()
		return nil, nil
	}, 0)

	run.Expect(t, "expected run")
	innerRun.Expect(t, "expected inner run")

	run = NewExpect()
	dep.Strobe()

	run.Expect(t, "expected rerun")
	// inner run is expected cache; if it runs, it will panic in calling Trigger
}

// TestRerunCache tests that a cached computation is rerun after it is invalidated.
func TestRerunCache(t *testing.T) {
	dep := NewResource()

	run := NewExpect()

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		Cache(ctx, 0, func(ctx context.Context) (interface{}, error) {
			AddDependency(ctx, dep)
			run.Trigger()
			return nil, nil
		})
		return nil, nil
	}, 0)

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
		AddDependency(ctx, dep)
		run.Trigger()
		return nil, nil
	}, 0)

	run.Expect(t, "expected run")

	runner.Stop()
	dep.Invalidate()

	// run is supposed to stop; if it runs, it will panic in calling Trigger
}

func TestError(t *testing.T) {
	dep := NewResource()

	run := NewExpect()

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		AddDependency(ctx, dep)
		run.Trigger()
		return nil, errors.New("error")
	}, 0)

	run.Expect(t, "expected run")

	dep.Invalidate()

	// run is supposed to stop; if it runs, it will panic in calling Trigger
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
	}, 0)

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
	}, 0)

	run.Expect(t, "expected run")
}

// TestMinRerunInterval tests that a runner debounces reruns
func TestMinRerunInterval(t *testing.T) {
	run := NewExpect()

	r := NewResource()
	var ran time.Time

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		AddDependency(ctx, r)
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
	}, 1*time.Second)

	run.Expect(t, "expected run")

	run = NewExpect()
	r.Strobe()
	run.Expect(t, "expected rerun")
}

// TestModifyCache tests that modifying the cache in debug mode causes a panic
func TestModifyCache(t *testing.T) {
	DebugCacheMutates = true
	defer func() { DebugCacheMutates = false }()

	run := NewExpect()

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		_, err := Cache(ctx, "outer", func(ctx context.Context) (interface{}, error) {
			ret, _ := Cache(ctx, "inner", func(ctx context.Context) (interface{}, error) {
				return map[string]string{"foo": "bar"}, nil
			})
			m := ret.(map[string]string)
			m["foo"] = "baz"
			return nil, nil
		})
		if err == nil || !strings.Contains(err.Error(), "cached value changed") {
			t.Error("expected err")
		}

		run.Trigger()

		return nil, nil
	}, 1*time.Second)

	run.Expect(t, "expected run")
}
