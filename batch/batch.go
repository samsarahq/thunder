// Package batch helps write efficient graphql resolvers with infrastructure
// for combining multiple RPCs into single batched RPCs. Batched RPCs are
// often more efficient that independent RPCs as they reduce the per-request
// overhead, but are difficult to use in graphql resolvers that only have
// a limited view of the overall query being computed.
//
// For example, when using a graphql query to fetch a list of users and the
// group each user is a part of, often the resolver that fetches the user's
// group is called once for each user, with no direct link to the other users.
// By default, this means the query will result in the number of users RPCs.
//
// This package's Func makes it easy to combine such independent invocations of
// a fetch function in a single batched RPC. Independent calls to Func.Invoke
// get automatically combined into a single call to the user-supplied Func.Many,
// resulting in only a single RPC with minimal changes to resolver code.
package batch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// A SingleFunc is a function that computes a result for a single input value.
type SingleFunc func(ctx context.Context, arg interface{}) (interface{}, error)

// A ManyFunc is a function that computes many results for many input values at once.
type ManyFunc func(ctx context.Context, args []interface{}) ([]interface{}, error)

// A ShardFunc is a function that computation a shard for a given input value.
type ShardFunc func(arg interface{}) interface{}

// DefaultMaxDuration is the default MaxDuration for Func.
var DefaultMaxDuration = 200 * time.Microsecond

// A Func transforms a ManyFunc into a SingleFunc (as Func.Invoke) that
// uses batching.
type Func struct {
	// Many is the required ManyFunc
	Many ManyFunc
	// Shard optionally splits different classes of arguments into independent
	// invocations of Many. For example, a Func that fetches rows from a SQL
	// database might shard by table so that each invocation of Many only has to
	// fetch rows from a single table.
	Shard ShardFunc
	// MaxSize optionally limits the size of a batch. After receiving MaxSize
	// invocations, Many will be invoked even if some goroutines are stil running.
	// Zero, the default, means no limit.
	MaxSize int
	// MaxDuration limits the duration of a batch. After waiting for
	// MaxDuration, Many will be invoked even if some goroutines are still
	// running. Defaults to DefaultMaxDuration.
	MaxDuration time.Duration
}

// A batchGroup prepares and tracks a single batched invocation of a Func.
type batchGroup struct {
	// args is the array of arguments to be passed to the Func.Many.
	args []interface{}
	// maxSizeCh is a 0-sized channel that is closed when len(args) hits Func.MaxSize.
	maxSizeCh chan struct{}
	// doneCh is a 0-sized channel that is closed once result and err are set.
	doneCh chan struct{}
	// result is an array of len(args) values with the result of the Func.
	result []interface{}
	// if err is nil, result is valid. Otherwise, err describes what went wrong.
	err error
}

// funcShard identifies a batchGroup for a given Func and result of Func.Shard.
type funcShard struct {
	f     *Func
	shard interface{}
}

// batchContext tracks context-specific batching information.
type batchContext struct {
	mu                 sync.Mutex
	pendingBatchGroups map[funcShard]*batchGroup
}

// batchContextKey is a context.Value key used for type *batchContext.
type batchContextKey struct{}

// WithBatching adds batching support to the given context.
func WithBatching(ctx context.Context) context.Context {
	if ctx.Value(batchContextKey{}) != nil {
		panic("WithBatching was already called on a parent context")
	}

	bctx := &batchContext{
		pendingBatchGroups: make(map[funcShard]*batchGroup),
	}
	return context.WithValue(ctx, batchContextKey{}, bctx)
}

// HasBatching returns if the given context has batching support.
func HasBatching(ctx context.Context) bool {
	return ctx.Value(batchContextKey{}) != nil
}

// safeInvoke invokes ManyFunc, recovering panics and handling the case when
// len(result) != len(args).
func safeInvoke(ctx context.Context, f ManyFunc, args []interface{}) (result []interface{}, err error) {
	defer func() {
		if p := recover(); p != nil {
			result = nil
			err = fmt.Errorf("ManyFunc paniced: %v", p)
		} else if err == nil && len(result) != len(args) {
			result = nil
			err = errors.New("ManyFunc returned incorrect number of results")
		}
	}()

	return f(ctx, args)
}

// Invoke arranges for the Func's Many to be called with arg as one of its
// arguments, and returns the corresponding result.
func (f *Func) Invoke(ctx context.Context, arg interface{}) (interface{}, error) {
	bctx := ctx.Value(batchContextKey{}).(*batchContext)
	if bctx == nil {
		panic("WithBatching must be called on the context before using Func")
	}

	// Determine the current Func shard.
	var shard interface{}
	if f.Shard != nil {
		shard = f.Shard(arg)
	}
	fs := funcShard{
		f:     f,
		shard: shard,
	}

	bctx.mu.Lock()
	// Look up the batchGroup for the Func shard, if any.
	bg, existed := bctx.pendingBatchGroups[fs]
	var timer *time.Timer
	if !existed {
		// If none, create a new one.
		bg = &batchGroup{
			doneCh: make(chan struct{}, 0),
		}
		if f.MaxSize > 0 {
			bg.maxSizeCh = make(chan struct{}, 0)
		}

		// Setup a MaxDuration timer.
		maxDuration := DefaultMaxDuration
		if f.MaxDuration > 0 {
			maxDuration = f.MaxDuration
		}
		timer = time.NewTimer(maxDuration)
		defer timer.Stop()

		// Publish the batchGroup.
		bctx.pendingBatchGroups[fs] = bg
	}

	// Add arg to the list of arguments to the batchGroup, and remember where to
	// find the result.
	index := len(bg.args)
	bg.args = append(bg.args, arg)

	// Maybe signal to run if we hit max batch size.
	if f.MaxSize > 0 && len(bg.args) == f.MaxSize {
		close(bg.maxSizeCh)
		delete(bctx.pendingBatchGroups, fs)
	}
	bctx.mu.Unlock()

	// Run the batchGroup if we created it. Otherwise, wait for the batchGroup to
	// finish.
	if !existed {
		// Wait for a trigger to run the batchGroup.
		select {
		case <-ctx.Done(): // Resolve if the context is canceled.
		case <-timer.C: // Resolve after a timeout to bound latency.
		case <-bg.maxSizeCh: // Resolve if we hit max batch size.
		}

		// Before we try and resolve, make sure noone will add to the group by
		// deleting it from the pending groups.
		bctx.mu.Lock()
		// Someone else might have already deleted us and started a new group if we
		// hit the maximum batch size; only delete ourselves.
		if bctx.pendingBatchGroups[fs] == bg {
			delete(bctx.pendingBatchGroups, fs)
		}
		bctx.mu.Unlock()

		// Check for the context being canceled.
		if ctx.Err() == nil {
			bg.result, bg.err = safeInvoke(ctx, f.Many, bg.args)
		} else {
			bg.err = ctx.Err()
		}
		// Make the result available.
		close(bg.doneCh)

	} else {
		// Wait for the result.
		<-bg.doneCh
	}

	// Return the local result.
	if bg.err != nil {
		return nil, bg.err
	}
	return bg.result[index], nil
}
