package batch

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/samsarahq/go/oops"
)

// WithCache associates a BatchCache with the context.
//
// Any future calls to BatchCache with this context will share a cache.
// WithCache should be called once per request to initialize a shared
// cache. The cache never evicts entries so it should be short-lived.
func WithCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, batchCacheKey{}, &cache{
		entries: make(map[interface{}]*cacheEntry),
	})
}

type batchCacheKey struct{}

// cache caches computations
type cache struct {
	mu      sync.Mutex
	entries map[interface{}]*cacheEntry
}

type cacheEntry struct {
	output interface{}
	doneCh chan struct{}
	done   uint32
	err    error
}

// A BatchMap should be a map[batch.Index]Value.
type BatchMap interface{}

// A BatchComputeFunc should be a
// func(context.Context, map[batch.Index]Input) (map[batch.Index]Output, error).
type BatchComputeFunc interface{}

type parsedBatchEntry struct {
	key        reflect.Value
	input      reflect.Value
	cacheEntry *cacheEntry
}

func (c *cache) fillEntries(inputBatchValue reflect.Value, doneCh chan struct{}) ([]parsedBatchEntry, []parsedBatchEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := make([]parsedBatchEntry, inputBatchValue.Len())
	computeIdx := 0
	waitIdx := len(entries) - 1

	iter := inputBatchValue.MapRange()
	for iter.Next() {
		mapKey := iter.Key()
		mapVal := iter.Value()

		cacheKey := mapVal.Interface() // XXX: make sure comparable?
		ce, ok := c.entries[cacheKey]
		if ok {
			entries[waitIdx] = parsedBatchEntry{
				key: mapKey,
				// input:      mapVal,
				cacheEntry: ce,
			}
			waitIdx--
			continue
		}

		ce = &cacheEntry{
			doneCh: doneCh,
		}
		entry := parsedBatchEntry{
			key:        mapKey,
			input:      mapVal,
			cacheEntry: ce,
		}
		c.entries[cacheKey] = ce
		entries[computeIdx] = entry
		computeIdx++
	}

	return entries[:computeIdx], entries
}

// Cache is the interface for a point cache that can run batch queries and
// cache the individual responses for each execution.
func Cache(ctx context.Context, inputBatch, outputBatch BatchMap, f BatchComputeFunc) error {
	inputBatchValue := reflect.ValueOf(inputBatch)
	outputBatchValue := reflect.ValueOf(outputBatch)

	fExecer, err := getFuncExecer(f)
	if err != nil {
		return err
	}

	batchCache, ok := ctx.Value(batchCacheKey{}).(*cache)
	if !ok {
		return oops.Errorf("missing batch cache in context")
	}

	doneCh := make(chan struct{}, 0)

	if err := fExecer.checkInputOutput(inputBatchValue.Type(), outputBatchValue.Type()); err != nil {
		return err
	}

	toCompute, all := batchCache.fillEntries(inputBatchValue, doneCh)

	if len(toCompute) > 0 {
		if err := fExecer.execute(ctx, toCompute); err != nil {
			for _, entry := range toCompute {
				entry.cacheEntry.err = err
				atomic.StoreUint32(&entry.cacheEntry.done, 1)
			}
		}
		close(doneCh)
	}

	for _, entry := range all {
		ce := entry.cacheEntry

		if atomic.LoadUint32(&ce.done) != 1 {
			select {
			case <-ce.doneCh:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if ce.err != nil {
			return ce.err
		}
		if ce.output != nil {
			// Value returned was nil or not there.
			outputBatchValue.SetMapIndex(entry.key, reflect.ValueOf(ce.output))
		}
	}

	return nil
}

type funcExecer struct {
	inputBatchType  reflect.Type
	outputBatchType reflect.Type
	funcVal         reflect.Value
}

func (f *funcExecer) checkInputOutput(inputBatchType, outputBatchType reflect.Type) error {
	if f.inputBatchType != inputBatchType {
		return oops.Errorf("function (%s) input batch type (%s) for func did not match provided batch type (%s)", f.funcVal.Type().String(), f.inputBatchType.String(), inputBatchType.String())
	}
	if f.outputBatchType != outputBatchType {
		return oops.Errorf("function (%s) output batch type (%s) for func did not match provided batch type (%s)", f.funcVal.Type().String(), f.outputBatchType.String(), outputBatchType.String())
	}
	return nil
}

var (
	errType        = reflect.TypeOf((*error)(nil)).Elem()
	contextType    = reflect.TypeOf((*context.Context)(nil)).Elem()
	batchIndexType = reflect.TypeOf(NewIndex(0))
)

func getFuncExecer(f interface{}) (*funcExecer, error) {
	funcVal := reflect.ValueOf(f)
	funcType := funcVal.Type()

	if funcType.NumIn() != 2 {
		return nil, oops.Errorf("expected input for batch func (%s) is (context map[batch.Index]Type), incorrect number of params", funcType.String())
	}
	if funcType.In(0) != contextType {
		return nil, oops.Errorf("expected input for batch func (%s) is (context map[batch.Index]Type), invalid context", funcType.String())
	}

	inputBatchFuncType := funcType.In(1)
	if inputBatchFuncType.Kind() != reflect.Map || inputBatchFuncType.Key() != batchIndexType {
		return nil, oops.Errorf("function (%s) input batch (%s) is not a map[batch.Index]Type", funcType.String(), inputBatchFuncType.String())
	}

	if funcType.NumOut() != 2 {
		return nil, oops.Errorf("expected output for batch func (%s) is (map[batch.Index]Type, error), incorrect number of results", funcType.String())
	}
	if funcType.Out(1) != errType {
		return nil, oops.Errorf("expected output for batch func (%s) is (map[batch.Index]Type, error), invalid error type", funcType.String())
	}

	outputBatchFuncType := funcType.Out(0)
	if outputBatchFuncType.Kind() != reflect.Map || outputBatchFuncType.Key() != batchIndexType {
		return nil, oops.Errorf("function (%s) output batch (%s) is not a map[batch.Index]Type", funcType.String(), outputBatchFuncType.String())
	}

	return &funcExecer{
		funcVal:         funcVal,
		inputBatchType:  inputBatchFuncType,
		outputBatchType: outputBatchFuncType,
	}, nil
}

func (f *funcExecer) execute(ctx context.Context, entries []parsedBatchEntry) error {
	newBatch := reflect.MakeMapWithSize(f.inputBatchType, len(entries))
	for _, entry := range entries {
		newBatch.SetMapIndex(entry.key, entry.input)
	}

	results := f.funcVal.Call([]reflect.Value{reflect.ValueOf(ctx), newBatch})

	if errVal := results[1]; !errVal.IsNil() {
		return errVal.Interface().(error)
	}

	resultVal := results[0]

	for _, entry := range entries {
		res := resultVal.MapIndex(entry.key)
		if !res.IsValid() || (res.Kind() == reflect.Ptr && res.IsNil()) {
			continue
		}
		entry.cacheEntry.output = res.Interface()
		atomic.StoreUint32(&entry.cacheEntry.done, 1)
	}

	return nil
}
