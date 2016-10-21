package thunder

import (
	"context"
	"sync"
	"time"
)

// locker is a collection of mutexes indexed by arbitrary keys
type locker struct {
	mu sync.Mutex
	m  map[interface{}]*lock
}

// newLocker creates a new locker instance.
func newLocker() *locker {
	return &locker{
		m: make(map[interface{}]*lock),
	}
}

// lock is a single mutex in a locker
type lock struct {
	ref int
	mu  sync.Mutex
}

// Lock locks a locker by (optionally) allocating, increasing the ref count,
// and locking
func (l *locker) Lock(k interface{}) {
	l.mu.Lock()
	m, ok := l.m[k]
	if !ok {
		m = new(lock)
		l.m[k] = m
	}
	m.ref++
	l.mu.Unlock()
	m.mu.Lock()
}

// Unlock unlocks a locker by unlocking, decreasing the ref count, and
// (optionally) deleting
func (l *locker) Unlock(k interface{}) {
	l.mu.Lock()
	m := l.m[k]
	m.mu.Unlock()
	m.ref--
	if m.ref == 0 {
		delete(l.m, k)
	}
	l.mu.Unlock()
}

type computation struct {
	node  node
	value interface{}
}

// cache caches computations
type cache struct {
	mu           sync.Mutex
	locker       *locker
	computations map[interface{}]*computation
}

func (c *cache) get(key interface{}) *computation {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.computations[key]
}

// set adds a computation to the cache for the given key
func (c *cache) set(key interface{}, computation *computation) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.computations[key] == nil {
		c.computations[key] = computation
	}
}

func (c *cache) cleanInvalidated() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, computation := range c.computations {
		if computation.node.Invalidated() {
			delete(c.computations, key)
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
		node: node{out: make(map[*node]struct{})},
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

func AddDependency(ctx context.Context, r *Resource) {
	if !HasRerunner(ctx) {
		r.node.addOut(&node{released: true})
		return
	}

	computation := ctx.Value(computationKey{}).(*computation)
	r.node.addOut(&computation.node)
}

type ComputeFunc func(context.Context) (interface{}, error)

func run(ctx context.Context, f ComputeFunc) (*computation, error) {
	// build result computation and local computation Ctx
	c := &computation{
		// this node will be freed either when the computation fails, or by our
		// caller
		node: node{out: make(map[*node]struct{})},
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

func Cache(ctx context.Context, key interface{}, f ComputeFunc) (interface{}, error) {
	if !HasRerunner(ctx) {
		return f(ctx)
	}

	cache := ctx.Value(cacheKey{}).(*cache)
	computation := ctx.Value(computationKey{}).(*computation)

	cache.locker.Lock(key)
	defer cache.locker.Unlock(key)

	if child := cache.get(key); child != nil {
		child.node.addOut(&computation.node)
		return child.value, nil
	}

	child, err := run(ctx, f)
	if err != nil {
		return nil, err
	}
	cache.set(key, child)

	child.node.addOut(&computation.node)
	return child.value, nil
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
			computations: make(map[interface{}]*computation),
			locker:       newLocker(),
		},
		minRerunInterval: minRerunInterval,
	}
	go r.run()
	return r
}

// run performs an actual computation
func (r *Rerunner) run() {
	delta := r.minRerunInterval - time.Now().Sub(r.lastRun)
	time.Sleep(delta)

	r.mu.Lock()
	defer r.mu.Unlock()

	// Bail out if the computation has been stopped.
	if r.stop {
		return
	}

	r.cache.cleanInvalidated()
	ctx := context.WithValue(r.ctx, cacheKey{}, r.cache)

	// Run f, and release the old computation right after.
	computation, err := run(ctx, r.f)
	if r.computation != nil {
		go r.computation.node.release()
		r.computation = nil
	}
	if err != nil {
		// If computation failed, stop the runner.
		return
	}

	// Store the computation.
	r.computation = computation

	r.lastRun = time.Now()

	// schedule a rerun whenever our node becomes invalidated (which might already
	// have happened!)
	computation.node.handleInvalidate(r.run)
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
