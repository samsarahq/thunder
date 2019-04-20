package reactive

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type batchArg struct {
	key string
	res batchRes
	dep *Resource
}

type batchRes struct {
	val string
}

func runBatchFunc(t *testing.T, ctx context.Context, source map[int]batchArg, expectedKeysInBatch []string) {
	res, err := BatchCache(
		ctx,
		func(i interface{}) interface{} {
			return i.(batchArg).key
		},
		func(i context.Context, batchMap2 BatchMap) (BatchMap, error) {
			batch := batchMap2.(map[int]batchArg)
			res := make(map[int]batchRes, len(batch))
			require.Equal(t, len(batch), len(expectedKeysInBatch), "expected vs actual batch sizes did not match")
			for idx, arg := range batch {
				AddDependency(ctx, arg.dep, nil)
				require.Contains(t, expectedKeysInBatch, arg.key)
				res[idx] = arg.res
			}
			return res, nil
		},
		source,
		map[int]batchRes{},
	)
	require.NoError(t, err)
	require.Len(t, res, len(source))
	for idx, res := range res.(map[int]batchRes) {
		require.Equal(t, source[idx].res.val, res.val)
	}
}

func TestBatchCache(t *testing.T) {
	allArgs := map[string]batchArg{
		"0": {key: "0", res: batchRes{val: "-0"}, dep: NewResource()},
		"1": {key: "1", res: batchRes{val: "-1"}, dep: NewResource()},
		"2": {key: "2", res: batchRes{val: "-2"}, dep: NewResource()},
		"3": {key: "3", res: batchRes{val: "-3"}, dep: NewResource()},
		"4": {key: "4", res: batchRes{val: "-4"}, dep: NewResource()},
		"5": {key: "5", res: batchRes{val: "-5"}, dep: NewResource()},
		"6": {key: "6", res: batchRes{val: "-6"}, dep: NewResource()},
	}
	run := NewExpect()

	batchSource1 := map[int]batchArg{
		0: allArgs["0"],
		1: allArgs["1"],
		2: allArgs["2"],
	}
	batchSource2 := map[int]batchArg{
		0: allArgs["2"],
		1: allArgs["3"],
		2: allArgs["4"],
		4: allArgs["1"],
	}
	batchSource3 := map[int]batchArg{
		0: allArgs["5"],
		1: allArgs["6"],
		2: allArgs["1"],
		4: allArgs["4"],
	}

	NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		runBatchFunc(t, ctx, batchSource1, []string{"0", "1", "2"})
		runBatchFunc(t, ctx, batchSource2, []string{"3", "4"})
		runBatchFunc(t, ctx, batchSource3, []string{"5", "6"})
		run.Trigger()

		return nil, nil
	}, 0)

	run.Expect(t, "expected run")
}
