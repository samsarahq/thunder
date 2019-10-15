package batch_test

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/samsarahq/thunder/batch"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

type batchArg struct {
	key string
	res batchRes
}

type batchRes struct {
	val string
}

func runBatchFuncWithExpectedKeys(t *testing.T, ctx context.Context, source map[batch.Index]batchArg, expectedKeysInBatch []int) {
	resultMap := make(map[batch.Index]batchRes, len(source))
	err := batch.Cache(ctx, source, resultMap,
		func(ctx context.Context, inBatch map[batch.Index]batchArg) (map[batch.Index]batchRes, error) {
			res := make(map[batch.Index]batchRes, len(inBatch))
			require.Equal(t, len(inBatch), len(expectedKeysInBatch), "expected vs actual batch sizes did not match")
			values := make([]int, 0, len(inBatch))
			for idx, arg := range inBatch {
				keyVal, err := strconv.Atoi(arg.key)
				require.NoError(t, err)
				values = append(values, keyVal)
				res[idx] = arg.res
			}
			sort.Ints(values)
			sort.Ints(expectedKeysInBatch)
			require.EqualValues(t, expectedKeysInBatch, values)
			return res, nil
		},
	)
	require.NoError(t, err)
	require.Len(t, resultMap, len(source))
	for idx, res := range resultMap {
		require.Equal(t, source[idx].res.val, res.val)
	}
}

func runBatchFuncAndGetRuns(t *testing.T, ctx context.Context, source map[batch.Index]batchArg) int {
	num := 0
	resultMap := make(map[batch.Index]batchRes, len(source))
	err := batch.Cache(ctx, source, resultMap,
		func(ctx context.Context, inBatch map[batch.Index]batchArg) (map[batch.Index]batchRes, error) {
			res := make(map[batch.Index]batchRes, len(inBatch))
			num += len(inBatch)
			for idx, arg := range inBatch {
				res[idx] = arg.res
			}
			return res, nil
		},
	)
	require.NoError(t, err)
	require.Len(t, resultMap, len(source))
	for idx, res := range resultMap {
		require.Equal(t, source[idx].res.val, res.val)
	}
	return num
}

func newBatchArg(i int) batchArg {
	pos := strconv.Itoa(i)
	neg := strconv.Itoa(-i)
	return batchArg{key: pos, res: batchRes{val: neg}}
}

func TestBatchCache(t *testing.T) {
	type request struct {
		batch        map[int]batchArg
		wantRequests []int
	}
	tests := []struct {
		name     string
		requests []request
	}{
		{
			name: "full re-query",
			requests: []request{
				{
					batch: map[int]batchArg{
						0: newBatchArg(0),
						1: newBatchArg(1),
						2: newBatchArg(2),
					},
					wantRequests: []int{0, 1, 2},
				},
				{
					batch: map[int]batchArg{
						0: newBatchArg(0),
						1: newBatchArg(1),
						2: newBatchArg(2),
					},
					wantRequests: []int{},
				},
			},
		},
		{
			name: "partial re-query",
			requests: []request{
				{
					batch: map[int]batchArg{
						0: newBatchArg(0),
						1: newBatchArg(1),
						2: newBatchArg(2),
					},
					wantRequests: []int{0, 1, 2},
				},
				{
					batch: map[int]batchArg{
						1:  newBatchArg(1),
						2:  newBatchArg(2),
						0:  newBatchArg(3),
						43: newBatchArg(4),
					},
					wantRequests: []int{3, 4},
				},
			},
		},
		{
			name: "partial re-query then re-query",
			requests: []request{
				{
					batch: map[int]batchArg{
						0: newBatchArg(0),
						1: newBatchArg(1),
						2: newBatchArg(2),
					},
					wantRequests: []int{0, 1, 2},
				},
				{
					batch: map[int]batchArg{
						0: newBatchArg(1),
						1: newBatchArg(2),
						2: newBatchArg(3),
						3: newBatchArg(4),
					},
					wantRequests: []int{3, 4},
				},
				{
					batch: map[int]batchArg{
						0: newBatchArg(2),
						1: newBatchArg(3),
						2: newBatchArg(6),
						3: newBatchArg(7),
					},
					wantRequests: []int{6, 7},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := batch.WithCache(context.Background())
			for _, req := range tt.requests {
				runBatchFuncWithExpectedKeys(t, ctx, toBatchMap(req.batch), req.wantRequests)
			}
		})
	}
}

func toBatchMap(orig map[int]batchArg) map[batch.Index]batchArg {
	newBatch := make(map[batch.Index]batchArg, len(orig))
	for idx, arg := range orig {
		newBatch[batch.NewIndex(idx)] = arg
	}
	return newBatch
}

func TestBatchCacheConcurrency(t *testing.T) {
	tests := []struct {
		name                string
		batches             []map[int]batchArg
		wantTotalExecutions int
	}{
		{
			name: "lotsa same queries",
			batches: []map[int]batchArg{
				{
					0: newBatchArg(0),
					1: newBatchArg(1),
					2: newBatchArg(2),
				},
				{
					0: newBatchArg(0),
					1: newBatchArg(1),
					2: newBatchArg(2),
				},
				{
					0: newBatchArg(0),
					1: newBatchArg(1),
					2: newBatchArg(2),
				},
				{
					0: newBatchArg(0),
					1: newBatchArg(1),
					2: newBatchArg(2),
				},
				{
					0: newBatchArg(0),
					1: newBatchArg(1),
					2: newBatchArg(2),
				},
				{
					0: newBatchArg(0),
					1: newBatchArg(1),
					2: newBatchArg(2),
				},
			},
			wantTotalExecutions: 3,
		},
		{
			name: "multi queries",
			batches: []map[int]batchArg{
				{
					0: newBatchArg(0),
					1: newBatchArg(1),
					2: newBatchArg(2),
				},
				{
					0: newBatchArg(3),
					1: newBatchArg(2),
					2: newBatchArg(1),
				},
				{
					0: newBatchArg(5),
					1: newBatchArg(6),
					2: newBatchArg(4),
				},
				{
					0: newBatchArg(4),
					1: newBatchArg(0),
					2: newBatchArg(3),
				},
				{
					0: newBatchArg(3),
					1: newBatchArg(2),
					2: newBatchArg(6),
				},
				{
					0: newBatchArg(2),
					1: newBatchArg(3),
					2: newBatchArg(4),
				},
			},
			wantTotalExecutions: 7,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := batch.WithCache(context.Background())
			var count int64
			g, ctx := errgroup.WithContext(ctx)
			for _, req := range tt.batches {
				reqVal := req
				g.Go(func() error {
					num := runBatchFuncAndGetRuns(t, ctx, toBatchMap(reqVal))
					atomic.AddInt64(&count, int64(num))
					return nil
				})
			}
			require.NoError(t, g.Wait())
			require.Equal(t, int64(tt.wantTotalExecutions), count)
		})
	}
}

func TestBatchCacheValidationErrors(t *testing.T) {
	tests := []struct {
		name                  string
		f                     batch.BatchComputeFunc
		sourceBatch           batch.BatchMap
		resultBatch           batch.BatchMap
		wantError             string
		wantResultStringBatch map[batch.Index]string
	}{
		{
			name:        "bad num params",
			f:           func(ctx context.Context) (map[batch.Index]string, error) { return nil, nil },
			sourceBatch: map[batch.Index]string{},
			resultBatch: map[batch.Index]string{},
			wantError:   "incorrect number of params",
		},
		{
			name:        "bad ctx type",
			f:           func(test string, mp map[batch.Index]string) (map[batch.Index]string, error) { return nil, nil },
			sourceBatch: map[batch.Index]string{},
			resultBatch: map[batch.Index]string{},
			wantError:   "invalid context",
		},
		{
			name:        "bad batch param",
			f:           func(ctx context.Context, mp map[string]string) (map[batch.Index]string, error) { return nil, nil },
			sourceBatch: map[string]string{},
			resultBatch: map[batch.Index]string{},
			wantError:   "is not a map[batch.Index]Type",
		},
		{
			name:        "inconsistent batch param",
			f:           func(ctx context.Context, mp map[batch.Index]int) (map[batch.Index]string, error) { return nil, nil },
			sourceBatch: map[batch.Index]string{},
			resultBatch: map[batch.Index]string{},
			wantError:   "did not match provided batch type",
		},
		{
			name:        "bad num resps",
			f:           func(ctx context.Context, mp map[batch.Index]string) map[batch.Index]string { return nil },
			sourceBatch: map[batch.Index]string{},
			resultBatch: map[batch.Index]string{},
			wantError:   "incorrect number of results",
		},
		{
			name:        "invalid error type",
			f:           func(ctx context.Context, mp map[batch.Index]string) (map[batch.Index]string, string) { return nil, "" },
			sourceBatch: map[batch.Index]string{},
			resultBatch: map[batch.Index]string{},
			wantError:   "invalid error type",
		},
		{
			name:        "bad batch result",
			f:           func(ctx context.Context, mp map[batch.Index]string) (map[string]string, error) { return nil, nil },
			sourceBatch: map[batch.Index]string{},
			resultBatch: map[string]string{},
			wantError:   "is not a map[batch.Index]Type",
		},
		{
			name:        "inconsistent batch result",
			f:           func(ctx context.Context, mp map[batch.Index]string) (map[batch.Index]string, error) { return nil, nil },
			sourceBatch: map[batch.Index]string{},
			resultBatch: map[batch.Index]int{},
			wantError:   "for func did not match provided batch type",
		},
		{
			name:        "no error from types",
			f:           func(ctx context.Context, mp map[batch.Index]string) (map[batch.Index]string, error) { return nil, nil },
			sourceBatch: map[batch.Index]string{},
			resultBatch: map[batch.Index]string{},
			wantError:   "",
		},
		{
			name: "error resp from func",
			f: func(ctx context.Context, mp map[batch.Index]string) (map[batch.Index]string, error) {
				return nil, errors.New("bad times")
			},
			sourceBatch: map[batch.Index]string{batch.NewIndex(0): "val"},
			resultBatch: map[batch.Index]string{},
			wantError:   "bad times",
		},
		{
			name: "return empty result",
			f: func(ctx context.Context, mp map[batch.Index]string) (map[batch.Index]string, error) {
				return nil, nil
			},
			sourceBatch:           map[batch.Index]string{batch.NewIndex(0): "val"},
			resultBatch:           map[batch.Index]string{},
			wantResultStringBatch: map[batch.Index]string{},
		},
		{
			name: "return subset result",
			f: func(ctx context.Context, mp map[batch.Index]string) (map[batch.Index]string, error) {
				result := make(map[batch.Index]string)
				for idx, val := range mp {
					if idx == batch.NewIndex(0) {
						continue
					}
					result[idx] = val
				}
				return result, nil
			},
			sourceBatch:           map[batch.Index]string{batch.NewIndex(0): "val", batch.NewIndex(1): "val2"},
			resultBatch:           map[batch.Index]string{},
			wantResultStringBatch: map[batch.Index]string{batch.NewIndex(1): "val2"},
		},
		{
			name: "return all results",
			f: func(ctx context.Context, mp map[batch.Index]string) (map[batch.Index]string, error) {
				result := make(map[batch.Index]string)
				for idx, val := range mp {
					result[idx] = val
				}
				return result, nil
			},
			sourceBatch:           map[batch.Index]string{batch.NewIndex(0): "val", batch.NewIndex(1): "val2"},
			resultBatch:           map[batch.Index]string{},
			wantResultStringBatch: map[batch.Index]string{batch.NewIndex(0): "val", batch.NewIndex(1): "val2"},
		},
		{
			name: "duplicate keys",
			f: func(ctx context.Context, mp map[batch.Index]string) (map[batch.Index]string, error) {
				result := make(map[batch.Index]string)
				for idx, val := range mp {
					result[idx] = val
				}
				return result, nil
			},
			sourceBatch:           map[batch.Index]string{batch.NewIndex(0): "val", batch.NewIndex(1): "val"},
			resultBatch:           map[batch.Index]string{},
			wantResultStringBatch: map[batch.Index]string{batch.NewIndex(0): "val", batch.NewIndex(1): "val"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := batch.Cache(batch.WithCache(context.Background()), tt.sourceBatch, tt.resultBatch, tt.f)
			if tt.wantError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)
			if tt.wantResultStringBatch != nil {
				resultBatch := tt.resultBatch.(map[batch.Index]string)
				require.Equal(t, len(tt.wantResultStringBatch), len(resultBatch))
				for idx, val := range tt.wantResultStringBatch {
					require.Equal(t, val, resultBatch[idx])
				}
			}
		})
	}
}
