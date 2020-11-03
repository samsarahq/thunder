package concurrencylimiter_test

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/northvolt/thunder/concurrencylimiter"
	"github.com/stretchr/testify/assert"
)

func TestStress(t *testing.T) {
	ctx := concurrencylimiter.With(context.Background(), 50)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10000; j++ {
				func() {
					ctx, release := concurrencylimiter.Acquire(ctx)
					defer release()
					concurrencylimiter.TemporarilyRelease(ctx, func() {
						runtime.Gosched()
					})
				}()
			}
		}()
	}
	wg.Wait()
}

// TestConcurrencyLimiter tests that the concurrency is limited.
func TestConcurrencyLimiter(t *testing.T) {
	ctx := concurrencylimiter.With(context.Background(), 2)

	var mu sync.Mutex
	count := 0
	maxCount := 0

	// Run 4 goroutines that sleep for 100ms, track how many are running at once.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			_, release := concurrencylimiter.Acquire(ctx)
			defer release()

			mu.Lock()
			count++
			if count > maxCount {
				maxCount = count
			}
			mu.Unlock()

			time.Sleep(100 * time.Millisecond)

			mu.Lock()
			count--
			mu.Unlock()
		}()
	}
	wg.Wait()
	assert.True(t, maxCount <= 2)
}

// TestNonLimitedTemporarilyRelease tests that calls to block allow other
// threads to run.
func TestNonLimitedTemporarilyRelease(t *testing.T) {
	ctx := concurrencylimiter.With(context.Background(), 2)

	var mu sync.Mutex
	count := 0
	maxCount := 0
	ran := 0
	any := false

	// Run 4 goroutines that sleep for 100ms, track how many are running at once.
	// The first calls sleep in TemporarilyRelease and should not be counted
	// against the concurrency limit.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			ctx, release := concurrencylimiter.Acquire(ctx)
			defer release()

			mu.Lock()
			count++
			if count > maxCount {
				maxCount = count
			}
			first := !any
			any = true
			mu.Unlock()

			f := func() {
				time.Sleep(100 * time.Millisecond)
				mu.Lock()
				ran++
				mu.Unlock()
			}

			if first {
				concurrencylimiter.TemporarilyRelease(ctx, f)
			} else {
				f()
			}

			mu.Lock()
			count--
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	assert.Equal(t, 3, maxCount)
	assert.Equal(t, 4, ran)
}

// TestDoubleTemporarilyRelease calls TemporarilyRelease twice on the same
// context. Both should run at the same time.
func TestDoubleTemporarilyRelease(t *testing.T) {
	ctx := concurrencylimiter.With(context.Background(), 1)
	ctx, release := concurrencylimiter.Acquire(ctx)
	defer release()

	var mu sync.Mutex
	count := 0
	maxCount := 0

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			concurrencylimiter.TemporarilyRelease(ctx, func() {
				mu.Lock()
				count++
				if count > maxCount {
					maxCount = count
				}
				mu.Unlock()

				time.Sleep(100 * time.Millisecond)

				mu.Lock()
				count--
				mu.Unlock()
			})
		}()
	}
	wg.Wait()

	assert.Equal(t, 2, maxCount)
}

// TestTemporarilyReleaseAfterRelease calls TemporarilyRelease after Release. It should run without
// problems.
func TestTemporarilyReleaseAfterRelease(t *testing.T) {
	ctx := concurrencylimiter.With(context.Background(), 1)
	ctx, release := concurrencylimiter.Acquire(ctx)
	release()

	ran := false

	concurrencylimiter.TemporarilyRelease(ctx, func() {
		ran = true
	})

	assert.True(t, ran)
}

// TestTemporarilyReleaseAfterRelease calls TemporarilyRelease without a
// Limiter. It should run without problems.
func TestTemporarilyReleaseWithoutLimit(t *testing.T) {
	ctx := context.Background()

	ran := false

	concurrencylimiter.TemporarilyRelease(ctx, func() {
		ran = true
	})

	assert.True(t, ran)
}

// TestReleaseDuringTemporarilyRelease calls Release and TemporarilyRelease in a
// race. It should not impact the running TemporarilyRelease.
func TestReleaseDuringTemporarilyRelease(t *testing.T) {
	for i := 0; i < 100; i++ {
		ctx := concurrencylimiter.With(context.Background(), 1)
		ctx, release := concurrencylimiter.Acquire(ctx)

		var wg sync.WaitGroup
		ran := false

		wg.Add(2)
		go func() {
			defer wg.Done()
			release()
		}()
		go func() {
			defer wg.Done()
			concurrencylimiter.TemporarilyRelease(ctx, func() {
				ran = true
			})
		}()
		wg.Wait()

		assert.True(t, ran)
	}
}

// TestAcquireContextCanceled tests that Acquire returns when a context is
// canceled.
func TestAcquireContextCanceled(t *testing.T) {
	ctx := concurrencylimiter.With(context.Background(), 0)

	ctx, cancel := context.WithCancel(ctx)
	cancel()

	ctx, release := concurrencylimiter.Acquire(ctx)
	release()
}

// TestAcquireReleaseNoLimiter tests that Acquire returns when the context
// has no limiter.
func TestAcquireReleaseNoLimiter(t *testing.T) {
	ctx := context.Background()

	ctx, release := concurrencylimiter.Acquire(ctx)
	release()
}
