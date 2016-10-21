package thunder

import "context"

// semaphore provides a set of tokens for limiting parallelism
type semaphore chan struct{}

func makeSemaphore(maxThreads int) semaphore {
	return make(chan struct{}, maxThreads)
}

func (s semaphore) acquire() {
	s <- struct{}{}
}

func (s semaphore) release() {
	<-s
}

// concurrencyLimiterKey is used as a key for a context.Context.
type concurrencyLimiterKey struct{}

// WithConcurrencyLimiter lets goroutines run with bounded parallelism
//
// The concurrency limiter tracks a limited set of goroutine tokens which a
// goroutine should acquire while doing work using AcquireGoroutineToken and
// ReleaseGoroutineToken.  Once the tokens are exhausted, AcquireGoroutineToken
// will block until another goroutine release its token using
// ReleaseGoroutineToken.
//
// Usually, a new goroutine should call
//
//   AcquireGoroutineToken(ctx)
//   defer ReleaseGoroutineToken(ctx)
//
// right after the start of the function. A function that blocks waiting on
// other goroutines should release its token before blocking to let other
// goroutines make progress, which might look like
//
//   ReleaseGoroutineToken(ctx)
//   ... block
//   AcquireGoroutineToken(ctx).
//
// Together, this combines into
//
//   ctx = WithConcurrencyLimiter(ctx, m)
//   var wg sync.WaitGroup
//   for i := 0; i < n; i++ {
//     wg.Add(1)
//     go func() {
//       defer wg.Done()
//       AcquireGoroutineToken(ctx)
//       defer ReleaseGoroutineToken(ctx)
//       ... do work
//     }()
//   }
//   ReleaseGoroutineToken(ctx)
//   wg.Wait()
//   AcquireGoroutineToken(ctx)
//
// to perform work in parallel with bounded parallelism.
func WithConcurrencyLimiter(ctx context.Context, maxThreads int) context.Context {
	semaphore := makeSemaphore(maxThreads)
	semaphore.acquire() // acquire one token for the main thread
	return context.WithValue(ctx, concurrencyLimiterKey{}, semaphore)
}

// AcquireGoroutineToken acquires a goroutine token
func AcquireGoroutineToken(ctx context.Context) {
	semaphore := ctx.Value(concurrencyLimiterKey{}).(semaphore)
	semaphore.acquire()
}

// ReleaseGoroutineToken releases a goroutine token
func ReleaseGoroutineToken(ctx context.Context) {
	semaphore := ctx.Value(concurrencyLimiterKey{}).(semaphore)
	semaphore.release()
}
