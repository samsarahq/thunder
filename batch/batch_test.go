package batch_test

import (
	"context"
	"sync"
	"testing"

	"time"

	"github.com/samsarahq/thunder/batch"
)

func TestBasic(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	f := batch.MakeBatchFunc(func(ctx context.Context, args []interface{}) ([]interface{}, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return args, nil
	})

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i // local copy

		wg.Add(1)
		batch.Go(ctx, func(ctx context.Context) {
			defer wg.Done()
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
		})
	}

	batch.BlockedOnOtherGoroutines(ctx, func(ctx context.Context) {
		wg.Wait()
	})

	if calls != 1 {
		t.Error(calls)
	}
}

func TestBackToBack(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	f := batch.MakeBatchFunc(func(ctx context.Context, args []interface{}) ([]interface{}, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return args, nil
	})

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i // local copy

		wg.Add(1)
		batch.Go(ctx, func(ctx context.Context) {
			defer wg.Done()
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
		})
	}

	batch.BlockedOnOtherGoroutines(ctx, func(ctx context.Context) {
		wg.Wait()
	})

	if calls != 2 {
		t.Error(calls)
	}
}

func TestShard(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	f := (&batch.BatchFunc{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return args, nil
		},
		Shard: func(arg interface{}) interface{} {
			return arg.(int) % 3
		},
	}).Invoke

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i // local copy

		wg.Add(1)
		batch.Go(ctx, func(ctx context.Context) {
			defer wg.Done()
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
		})
	}

	batch.BlockedOnOtherGoroutines(ctx, func(ctx context.Context) {
		wg.Wait()
	})

	// Expect 3 calls, one for each shard.
	if calls != 3 {
		t.Error(calls)
	}
}

func TestMaxSize(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	f := (&batch.BatchFunc{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return args, nil
		},
		MaxSize: 5,
	}).Invoke

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i // local copy

		wg.Add(1)
		batch.Go(ctx, func(ctx context.Context) {
			defer wg.Done()
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
		})
	}

	batch.BlockedOnOtherGoroutines(ctx, func(ctx context.Context) {
		wg.Wait()
	})

	// Expect 4 calls, one for each shard.
	if calls != 4 {
		t.Error(calls)
	}
}

func TestMaxDuration(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	f := (&batch.BatchFunc{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return args, nil
		},
		MaxDuration: 50 * time.Microsecond,
	}).Invoke

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i // local copy

		wg.Add(1)
		batch.Go(ctx, func(ctx context.Context) {
			defer wg.Done()
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
		})
	}

	// Wait for completion without invoking batch.BlockedOnOtherGoroutines.
	wg.Wait()

	// Expect 1 call, but tolerate up to 2 (because of races).
	if calls > 2 {
		t.Error(calls)
	}
}
