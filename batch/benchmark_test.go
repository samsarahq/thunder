package batch_test

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/samsarahq/thunder/batch"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

var durationMsOpts = []int64{
	1,
	10,
	100,
}

var requestRunOpts = []struct {
	name                    string
	numPossibleSelections   int
	numSelectionsPerRequest int
	numRequests             int
}{
	{
		name:                    "simple",
		numPossibleSelections:   10,
		numSelectionsPerRequest: 10,
		numRequests:             10,
	},
	{
		name:                    "small high overlap",
		numPossibleSelections:   20,
		numSelectionsPerRequest: 10,
		numRequests:             100,
	},
	{
		name:                    "small low overlap",
		numPossibleSelections:   100,
		numSelectionsPerRequest: 10,
		numRequests:             100,
	},
	{
		name:                    "large high overlap",
		numPossibleSelections:   200,
		numSelectionsPerRequest: 100,
		numRequests:             100,
	},
	{
		name:                    "large low overlap",
		numPossibleSelections:   1000,
		numSelectionsPerRequest: 100,
		numRequests:             100,
	},
}

func BenchmarkBatchCache_NoCachingCallAtAll(b *testing.B) {
	ctxFunc := func() context.Context { return context.Background() }
	for _, opts := range requestRunOpts {
		for _, durOpts := range durationMsOpts {
			b.Run(fmt.Sprintf("%s-%dMs", opts.name, durOpts), func(b *testing.B) {
				runBenchmark(
					b,
					ctxFunc,
					opts.numPossibleSelections,
					opts.numSelectionsPerRequest,
					opts.numRequests,
					durOpts,
					false,
				)
			})
		}
	}
}
func BenchmarkBatchCache_BatchButNoCaching(b *testing.B) {
	ctxFunc := func() context.Context { return context.Background() }
	for _, opts := range requestRunOpts {
		for _, durOpts := range durationMsOpts {
			b.Run(fmt.Sprintf("%s-%dMs", opts.name, durOpts), func(b *testing.B) {
				runBenchmark(
					b,
					ctxFunc,
					opts.numPossibleSelections,
					opts.numSelectionsPerRequest,
					opts.numRequests,
					durOpts,
					true,
				)
			})
		}
	}
}

func BenchmarkBatchCache_WithCaching(b *testing.B) {
	ctxFunc := func() context.Context { return batch.WithCache(context.Background()) }
	for _, opts := range requestRunOpts {
		for _, durOpts := range durationMsOpts {
			b.Run(fmt.Sprintf("%s-%dMs", opts.name, durOpts), func(b *testing.B) {
				runBenchmark(
					b,
					ctxFunc,
					opts.numPossibleSelections,
					opts.numSelectionsPerRequest,
					opts.numRequests,
					durOpts,
					true,
				)
			})
		}
	}
}

func runBenchmark(b *testing.B, ctxFunc func() context.Context, numPossibleSelections int, numSelectionsPerRequest int, numRequests int, calculationExpenseMs int64, callBatch bool) {
	rand.Seed(100)
	allArgs := make(map[int]batchArg, numPossibleSelections)
	for i := 0; i < numPossibleSelections; i++ {
		iStr := strconv.Itoa(i)
		allArgs[i] = batchArg{key: iStr, res: batchRes{val: iStr}}
	}
	requests := make([]map[batch.Index]batchArg, numRequests)
	for i := 0; i < numRequests; i++ {
		requests[i] = make(map[batch.Index]batchArg, numSelectionsPerRequest)
		for j := 0; j < numSelectionsPerRequest; j++ {
			requests[i][batch.NewIndex(j)] = allArgs[rand.Int()%numPossibleSelections]
		}
	}
	callFunc := func(ctx context.Context, inBatch map[batch.Index]batchArg) (map[batch.Index]batchRes, error) {
		time.Sleep(time.Duration(calculationExpenseMs) * time.Millisecond) // Force sleep
		res := make(map[batch.Index]batchRes, len(inBatch))
		for idx, arg := range inBatch {
			res[idx] = arg.res
		}
		return res, nil
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := ctxFunc()
		g, ctx := errgroup.WithContext(ctx)
		for j := 0; j < numRequests; j++ {
			jval := j
			g.Go(func() error {
				if !callBatch {
					resultMap, err := callFunc(ctx, requests[jval])
					require.Equal(b, len(requests[jval]), len(resultMap))
					return err
				}
				resultMap := make(map[batch.Index]batchRes, numSelectionsPerRequest)
				err := batch.Cache(ctx, requests[jval], resultMap, callFunc)
				require.Equal(b, len(requests[jval]), len(resultMap))
				return err
			})
		}
		require.NoError(b, g.Wait())
	}
}
