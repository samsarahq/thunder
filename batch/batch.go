package batch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// A Func is a function that computes a result for a single input value.
type Func func(ctx context.Context, arg interface{}) (interface{}, error)

// A ManyFunc is a function that computes many results for many input values at once.
type ManyFunc func(ctx context.Context, args []interface{}) ([]interface{}, error)

// A ShardFunc is a function that computation a shard for a given input value.
type ShardFunc func(arg interface{}) interface{}

// A Batcher transforms a ManyFunc into a Func (as BatchFunc.Invoke) that
// uses batching.
type BatchFunc struct {
	// Many is the required ManyFunc
	Many ManyFunc

	// Shard optionally splits different classes of arguments into independent
	// invocations of Many. For example, a BatchFunc that fetches rows from
	// a SQL database might shard by table so that each invocation of Many
	// only has to fetch rows from a single table.
	Shard ShardFunc

	// MaxSize optionally limits the size of a batch. After receiving MaxSize
	// invocations, Many will be invoked even if some goroutines are stil running.
	MaxSize int

	// MaxDuration optionally limits the duration of a batch. After waiting for
	// MaxDuration, Many will be invoked even if some goroutines are still
	// running.
	MaxDuration time.Duration
}

// MakeShardedBatchingFunc transforms a ManyFunc into a Func using batching and
// sharding.
//
// Individual invocations of the returned Func on the same shard will be
// combined into few
// invocations of the given ManyFunc.
func MakeShardedBatchingFunc(many ManyFunc, shard ShardFunc) Func {
	bf := &BatchFunc{
		Many:  many,
		Shard: shard,
	}
	return bf.Invoke
}

// MakeBatchFunc transforms a ManyFunc into a Func using batching.
//
// Individual invocations of the returned Func will be combined into few
// invocations of the given ManyFunc.
func MakeBatchFunc(many ManyFunc) Func {
	return (&BatchFunc{Many: many}).Invoke
}

type batchGroupKey struct {
	batchFunc *BatchFunc
	shard     interface{}
}

type batchGroup struct {
	args      []interface{}
	maxSizeCh chan struct{}
	timer     *time.Timer

	doneCh chan struct{}
	result []interface{}
	err    error
}

// batchContext tracks context-specific batching information.
type batchContext struct {
	mu sync.Mutex

	// batchGroups tracks the current pending BatchFunc invocations, with one
	// group per BatchFunc shard.
	batchGroups map[batchGroupKey]*batchGroup

	// numRunning is the number of non-paused goroutines.
	numRunning int

	// allPaused is a signal chan that will be closed when numRunning hits 0.
	allPaused chan struct{}
}

// batchContextKey is a context.Value key used for type *batchContext.
type batchContextKey struct{}

// WithBatching adds batching support to the given context.
//
// Batching requires careful instrumentation of all goroutines. This context is
// unique to the running goroutine, and you may not pass this context to
// another goroutine. Instead, start new child goroutines by calling Go.
func WithBatching(ctx context.Context) context.Context {
	return context.WithValue(ctx, batchContextKey{}, &batchContext{
		numRunning:  1,
		allPaused:   make(chan struct{}, 0),
		batchGroups: make(map[batchGroupKey]*batchGroup),
	})
}

func (bc *batchContext) start() {
	bc.mu.Lock()
	bc.numRunning++
	bc.mu.Unlock()
}

func (bc *batchContext) stop() chan struct{} {
	bc.mu.Lock()
	bc.numRunning--
	if bc.numRunning < 0 {
		panic(bc.numRunning)
	}
	allPaused := bc.allPaused
	if bc.numRunning == 0 {
		close(bc.allPaused)
		bc.allPaused = make(chan struct{}, 0)
	}
	bc.mu.Unlock()
	return allPaused
}

// Go starts a new goroutine running f. To use Go, you must have first called
// WithBatching to add batchContext to the context.
//
// The new goroutine starts with its own unique context, and you may not pass
// this context to another goroutine. Instead, start new child goroutines by
// calling Go again.
func Go(ctx context.Context, f func(ctx context.Context)) {
	bc := ctx.Value(batchContextKey{}).(*batchContext)

	bc.start()
	go func() {
		defer bc.stop()
		f(ctx)
	}()
}

// BlockedOnOtherGoroutines marks the current Goroutine as waiting on other
// goroutines while f is running.
func BlockedOnOtherGoroutines(ctx context.Context, f func(ctx context.Context)) {
	bc := ctx.Value(batchContextKey{}).(*batchContext)

	bc.stop()
	f(ctx)
	bc.start()
}

func safeInvoke(ctx context.Context, f ManyFunc, args []interface{}) (result []interface{}, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("ManyFunc paniced: %v", p)
			return
		}

		if err == nil && len(result) != len(args) {
			err = errors.New("ManyFunc returned incorrect number of results")
			return
		}
	}()

	return f(ctx, args)
}

func fulfill(ctx context.Context, bc *batchContext, k batchGroupKey, bf *BatchFunc, bg *batchGroup) {
	allPausedCh := bc.stop()

	var timerCh <-chan time.Time
	if bg.timer != nil {
		timerCh = bg.timer.C
	}

	select {
	// Always resolve when all goroutines have paused.
	case <-allPausedCh:
		break
	// Resolve after a timeout to bound latency.
	case <-timerCh:
		break
	// Resolve if we hit max batch size.
	case <-bg.maxSizeCh:
		break
	// Resolve if the context is canceled.
	case <-ctx.Done():
		bg.err = ctx.Err()
		bc.mu.Lock()
		bc.numRunning += len(bg.args)
		bc.mu.Unlock()
		close(bg.doneCh)
		return
	}

	if bg.timer != nil {
		bg.timer.Stop()
	}

	bc.mu.Lock()
	// Someone else might have already started a new batch group (for example, if we hit the maximum batch size.)
	if bc.batchGroups[k] == bg {
		bc.batchGroups[k] = nil
	}
	bc.mu.Unlock()

	bg.result, bg.err = safeInvoke(ctx, bf.Many, bg.args)
	bc.mu.Lock()
	bc.numRunning += len(bg.args)
	bc.mu.Unlock()
	close(bg.doneCh)
}

// Invoke arranges for the BatchFunc's Many to be called with arg as one of its
// arguments, and returns the corresponding result.
func (bf *BatchFunc) Invoke(ctx context.Context, arg interface{}) (interface{}, error) {
	bc := ctx.Value(batchContextKey{}).(*batchContext)

	var shard interface{}
	if bf.Shard != nil {
		shard = bf.Shard(arg)
	}

	k := batchGroupKey{
		batchFunc: bf,
		shard:     shard,
	}

	bc.mu.Lock()
	bg, existed := bc.batchGroups[k]
	if !existed {
		bg = &batchGroup{
			doneCh: make(chan struct{}, 0),
		}
		if bf.MaxSize > 0 {
			bg.maxSizeCh = make(chan struct{}, 0)
		}
		if bf.MaxDuration > 0 {
			bg.timer = time.NewTimer(bf.MaxDuration)
		}
		bc.batchGroups[k] = bg
	}
	index := len(bg.args)
	bg.args = append(bg.args, arg)

	if bf.MaxSize > 0 && len(bg.args) == bf.MaxSize {
		close(bg.maxSizeCh)
		bc.batchGroups[k] = nil
	}

	bc.mu.Unlock()

	if existed {
		bc.stop()
		<-bg.doneCh
	} else {
		fulfill(ctx, bc, k, bf, bg)
	}

	if bg.err != nil {
		return nil, bg.err
	}
	return bg.result[index], nil
}

// TODO: panic on multiple calls to stop() with the same ctx
