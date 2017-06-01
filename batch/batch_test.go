package batch_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/samsarahq/thunder/batch"
)

// TestBasic tests that batch.Func with default options batches calls.
func TestBasic(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	f := (&batch.Func{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return args, nil
		},
	}).Invoke

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
		}(i)
	}
	wg.Wait()

	// Expect 1, allow for 2 in case of races.
	if calls > 2 {
		t.Error(calls)
	}
}

// TestWaitInterval tests that the function invocation is delayed while we
// consistently invoke the batch function.
func TestWaitInterval(t *testing.T) {
	const loopCount = 20

	testcases := []struct {
		description   string
		interval      time.Duration
		maxDuration   time.Duration
		sleepDuration time.Duration
		allowedCounts map[int]bool
	}{
		{
			description:   "Expect sleep less than WaitDuration to result in a single call being made.",
			interval:      500 * time.Microsecond,
			sleepDuration: 100 * time.Microsecond,
			maxDuration:   10 * time.Millisecond,
			allowedCounts: map[int]bool{1: true},
		},
		{
			description:   "Expect sleep greater than WaitDuration to result in loopCount calls being made.",
			interval:      500 * time.Microsecond,
			sleepDuration: 700 * time.Microsecond,
			maxDuration:   10 * time.Millisecond,
			allowedCounts: map[int]bool{loopCount: true, loopCount - 1: true}, // Expect loopCount, allow loopCount-1 for race conditions.
		},
		{
			description:   "Expect sleep less than than WaitDuration but aggregate over MaxDuration to result in 2 calls being made.",
			interval:      500 * time.Microsecond,
			sleepDuration: 60 * time.Microsecond,
			maxDuration:   1 * time.Millisecond,
			allowedCounts: map[int]bool{2: true, 3: true}, // Expect 2, allow 3 for race conditions.
		},
	}

	for i, testcase := range testcases {
		var mu sync.Mutex
		count := 0
		f := (&batch.Func{
			WaitInterval: testcase.interval,
			MaxDuration:  testcase.maxDuration,
			Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
				mu.Lock()
				defer mu.Unlock()
				count++
				return args, nil
			},
		}).Invoke

		ctx := batch.WithBatching(context.Background())

		var wg sync.WaitGroup
		for i := 0; i < loopCount; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if result, err := f(ctx, i); err != nil || result != i {
					t.Error(err, i)
				}
			}(i)
			time.Sleep(testcase.sleepDuration)
		}
		wg.Wait()

		if !testcase.allowedCounts[count] {
			t.Errorf("Test %d: %s: Allowable=%v, Actual=%v", i, testcase.description, testcase.allowedCounts, count)
		}
	}
}

// TestBackToBack tests that two back-to-back invocations of batch.Func from
// multiple goroutines get batched in a total of two calls.
func TestBackToBack(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	f := (&batch.Func{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return args, nil
		},
	}).Invoke

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
		}(i)
	}
	wg.Wait()

	// Expect 2, allow for 3 in case of races.
	if calls > 3 {
		t.Error(calls)
	}
}

// TestShard tests that Func.Shard shards invocations according to the shard.
func TestShard(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	f := (&batch.Func{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			mu.Lock()
			defer mu.Unlock()
			for _, i := range args {
				if i.(int)%3 != args[0].(int)%3 {
					return nil, errors.New("bad shard")
				}
			}
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
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
		}(i)
	}
	wg.Wait()

	// Expect 3 calls, one for each shard, allow for 4 in case of races.
	if calls > 4 {
		t.Error(calls)
	}
}

// TestMaxSize tests that no more than Func.MaxSize arguments get batched
// together.
func TestMaxSize(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	f := (&batch.Func{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			mu.Lock()
			defer mu.Unlock()
			if len(args) > 5 {
				return nil, errors.New("too many")
			}
			calls++
			return args, nil
		},
		MaxSize: 5,
	}).Invoke

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if result, err := f(ctx, i); err != nil || result != i {
				t.Error(err, i)
			}
		}(i)
	}
	wg.Wait()

	// Expect 4 calls, one for each shard, allow for 5 in case of races.
	if calls > 5 {
		t.Error(calls)
	}
}

// TestIncorrectNumberOfResults tests that a batch function that returns the
// wrong number of results is handled gracefully.
func TestIncorrectNumberOfResults(t *testing.T) {
	f := (&batch.Func{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			return append(args, nil), nil
		},
	}).Invoke

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := f(ctx, i); err == nil || !strings.Contains(err.Error(), "incorrect number of results") {
				t.Error(err, i)
			}
		}(i)
	}
	wg.Wait()
}

// TestPanic tests that a batch function that panics is handled gracefully.
func TestPanic(t *testing.T) {
	f := (&batch.Func{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			panic("foo")
		},
	}).Invoke

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := f(ctx, i); err == nil || !strings.Contains(err.Error(), "panicked: foo") {
				t.Error(err, i)
			}
		}(i)
	}
	wg.Wait()
}

// TestError tests that a batch function that returns an error is handled correctly.
func TestError(t *testing.T) {
	f := (&batch.Func{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			return nil, errors.New("some error")
		},
	}).Invoke

	ctx := batch.WithBatching(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := f(ctx, i); err == nil || !strings.Contains(err.Error(), "some error") {
				t.Error(err, i)
			}
		}(i)
	}
	wg.Wait()
}
