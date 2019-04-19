package reactive

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// Sentinel error to tell the rerunner to not dump the current
	// computation cache and let the error'd function retry.
	RetrySentinelError = errors.New("retry")

	// WriteThenReadDelay is how long to wait after hearing a change
	// was made, before reading that change by rerunning.
	WriteThenReadDelay = 200 * time.Millisecond
)

type computation struct {
	node  node
	value interface{}
}

type cacheEntry struct {
	done        chan struct{}
	err         error
	computation *computation
}

// cache caches computations
type cache struct {
	mu      sync.Mutex
	entries map[interface{}]*cacheEntry
}

func (c *cache) cleanInvalidated() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		select {
		case <-entry.done:
			if entry.computation.node.Invalidated() {
				delete(c.entries, key)
			}
		default:
			// orphaned computation still running?
			delete(c.entries, key)
			continue
		}
	}
}

// Resource represents a leaf-level dependency in a computation
type Resource struct {
	node
}

// NewResource creates a new Resource
func NewResource() *Resource {
	return &Resource{
		node: node{},
	}
}

// Invalidate permanently invalidates r
func (r *Resource) Invalidate() {
	go r.invalidate()
}

// Store invalidates all computations currently depending on r
func (r *Resource) Strobe() {
	go r.strobe()
}

// Cleanup registers a handler to be called when all computations using r stop
//
// NOTE: For f to be called, at least one computation must AddDependency r!
func (r *Resource) Cleanup(f func()) {
	r.node.handleRelease(f)
}

type computationKey struct{}
type cacheKey struct{}

type dependencySetKey struct{}

type dependencySet struct {
	mu           sync.Mutex
	dependencies []Dependency
}

func (ds *dependencySet) add(dep Dependency) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.dependencies = append(ds.dependencies, dep)
}

func (ds *dependencySet) get() []Dependency {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.dependencies
}

type Dependency interface{}

type DependencyCallbackFunc func(context.Context, Dependency)

type dependencyCallbackKey struct{}

func AddDependency(ctx context.Context, r *Resource, dep Dependency) {
	if !HasRerunner(ctx) {
		r.node.addOut(&node{released: true})
		return
	}

	computation := ctx.Value(computationKey{}).(*computation)
	r.node.addOut(&computation.node)

	if dep != nil {
		depSet, ok := ctx.Value(dependencySetKey{}).(*dependencySet)
		if ok && depSet != nil {
			depSet.add(dep)
		}
		if callback, ok := ctx.Value(dependencyCallbackKey{}).(DependencyCallbackFunc); ok && callback != nil {
			callback(ctx, dep)
		}
	}
}

// WithDependencyCallback registers a callback that is invoked when
// AddDependency is called with non-nil serializable dependency.
func WithDependencyCallback(ctx context.Context, f DependencyCallbackFunc) context.Context {
	return context.WithValue(ctx, dependencyCallbackKey{}, f)
}

func Dependencies(ctx context.Context) []Dependency {
	depSet := ctx.Value(dependencySetKey{}).(*dependencySet)
	if depSet == nil {
		return nil
	}
	return depSet.get()
}

type ComputeFunc func(context.Context) (interface{}, error)

func run(ctx context.Context, f ComputeFunc) (*computation, error) {
	// build result computation and local computation Ctx
	c := &computation{
		// this node will be freed either when the computation fails, or by our
		// caller
		node: node{},
	}

	childCtx := context.WithValue(ctx, computationKey{}, c)

	// Compute f and write the results to the c
	value, err := f(childCtx)
	if err != nil {
		go c.node.release()
		return nil, err
	}

	c.value = value

	return c, nil
}

type BatchMap interface{} // map[int]Something
type BatchComputeFunc func(context.Context, BatchMap) (BatchMap, error)

func BatchCache(ctx context.Context, keyFunc func(interface{}) interface{}, f BatchComputeFunc, batch BatchMap) (BatchMap, error) {
}

func Cache(ctx context.Context, key interface{}, f ComputeFunc) (interface{}, error) {
	if !HasRerunner(ctx) {
		val, err := f(ctx)
		return val, err
	}

	cache := ctx.Value(cacheKey{}).(*cache)
	parent := ctx.Value(computationKey{}).(*computation)

	cache.mu.Lock()
	entry, ok := cache.entries[key]
	if !ok {
		// First cache call for this key.
		entry = &cacheEntry{
			done: make(chan struct{}),
		}
		cache.entries[key] = entry
	}
	cache.mu.Unlock()

	if ok {
		// The computation for key is running or has finished.
		select {
		case <-entry.done:
			if entry.computation == nil {
				// If previous f() panics, computation and err are not set.
				return nil, errors.New("previous computation failed")
			}
			if entry.err != nil {
				return nil, entry.err
			}
			entry.computation.node.addOut(&parent.node)
			return entry.computation.value, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	defer close(entry.done)

	entry.computation, entry.err = run(ctx, f)
	if entry.err != nil {
		return nil, entry.err
	}

	entry.computation.node.addOut(&parent.node)
	return entry.computation.value, nil
}

// Rerunner automatically reruns a computation whenever its dependencies
// change.
//
// The computation stops when it returns an error or after calling Stop.  There
// is no way to get the output value from a computation. Instead, the
// computation should communicate its result before returning.
type Rerunner struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	f                ComputeFunc
	cache            *cache
	minRerunInterval time.Duration
	retryDelay       time.Duration

	// flushed tracks if the next computation should run without delay. It is set
	// to false as soon as the next computation starts. flushCh is closed when
	// flushed is set to true.
	flushMu sync.Mutex
	flushCh chan struct{}
	flushed bool

	mu          sync.Mutex
	computation *computation
	stop        bool

	lastRun time.Time
}

// NewRerunner runs f continuously
func NewRerunner(ctx context.Context, f ComputeFunc, minRerunInterval time.Duration) *Rerunner {
	ctx, cancelCtx := context.WithCancel(ctx)

	r := &Rerunner{
		ctx:       ctx,
		cancelCtx: cancelCtx,

		f: f,
		cache: &cache{
			entries: make(map[interface{}]*cacheEntry),
		},
		minRerunInterval: minRerunInterval,
		retryDelay:       minRerunInterval,

		flushCh: make(chan struct{}, 0),
	}
	go r.run()
	return r
}

// RerunImmediately removes the delay from the next recomputation.
func (r *Rerunner) RerunImmediately() {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()

	if !r.flushed {
		close(r.flushCh)
		r.flushed = true
	}
}

// run performs an actual computation
func (r *Rerunner) run() {
	// Wait for the minimum rerun interval. Exit early if the computation is stopped.
	delta := r.retryDelay - time.Now().Sub(r.lastRun)

	t := time.NewTimer(delta)
	select {
	case <-r.ctx.Done():
	case <-t.C:
	case <-r.flushCh:
	}
	t.Stop()
	if r.ctx.Err() != nil {
		return
	}

	r.flushMu.Lock()
	if r.flushed {
		r.flushCh = make(chan struct{}, 0)
		r.flushed = false
	}
	r.flushMu.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Bail out if the computation has been stopped.
	if r.stop {
		return
	}

	if !r.lastRun.IsZero() {
		// Delay the rerun in order to emulate write-then-read consistency.
		time.Sleep(WriteThenReadDelay)
	}
	r.cache.cleanInvalidated()
	ctx := context.WithValue(r.ctx, cacheKey{}, r.cache)
	ctx = context.WithValue(ctx, dependencySetKey{}, &dependencySet{})

	computation, err := run(ctx, r.f)
	r.lastRun = time.Now()
	if err != nil {
		if err == RetrySentinelError {
			r.retryDelay = r.retryDelay * 2

			// Max out the retry delay to at 1 minute.
			if r.retryDelay > time.Minute {
				r.retryDelay = time.Minute
			}
			go r.run()
		} else {
			// If we encountered an error that is not the retry sentinel,
			// we should stop the rerunner.
			return
		}
	} else {
		// If we succeeded in the computation, we can release the old computation
		// and reset the retry delay.
		if r.computation != nil {
			go r.computation.node.release()
			r.computation = nil
		}

		r.computation = computation
		r.retryDelay = r.minRerunInterval

		// Schedule a rerun whenever our node becomes invalidated (which might already
		// have happened!)
		computation.node.handleInvalidate(r.run)
	}
}

func (r *Rerunner) Stop() {
	// Call cancelCtx before acquiring the lock as the lock might be held for a long time during a running computation.
	r.cancelCtx()

	r.mu.Lock()
	r.stop = true
	if r.computation != nil {
		go r.computation.node.release()
		r.computation = nil
	}
	r.mu.Unlock()
}

func HasRerunner(ctx context.Context) bool {
	return ctx.Value(computationKey{}) != nil
}
