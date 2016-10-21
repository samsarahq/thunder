package graphql

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sync"

	"github.com/hashicorp/go-multierror"

	"github.com/samsarahq/thunder"
)

// PrepareQuery checks that the given selectionSet matches the schema typ, and
// parses the args in selectionSet
func PrepareQuery(typ Type, selectionSet *SelectionSet) error {
	switch typ := typ.(type) {
	case *Scalar:
		if selectionSet != nil {
			return NewSafeError("scalar field must have no selections")
		}
		return nil

	case *Object:
		if selectionSet == nil {
			return NewSafeError("object field must have selections")
		}
		for _, selection := range selectionSet.Selections {
			field, ok := typ.Fields[selection.Name]
			if !ok {
				return NewSafeError(`unknown field "%s"`, selection.Name)
			}

			parsed, err := field.ArgParser.Parse(selection.Args)
			if err != nil {
				return NewSafeError(`error parsing args for "%s": %s`, selection.Name, err)
			}
			selection.Args = parsed

			if err := PrepareQuery(field.Type, selection.SelectionSet); err != nil {
				return err
			}
		}
		for _, fragment := range selectionSet.Fragments {
			if err := PrepareQuery(typ, fragment.SelectionSet); err != nil {
				return err
			}
		}
		return nil

	case *List:
		return PrepareQuery(typ.Type, selectionSet)

	default:
		panic("unknown type kind")
	}
}

type panicError struct {
	message string
}

func (p panicError) Error() string {
	return p.message
}

func safeResolve(ctx context.Context, field *Field, source, args interface{}, selectionSet *SelectionSet) (result interface{}, err error) {
	defer func() {
		if panicErr := recover(); panicErr != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			result, err = nil, fmt.Errorf("graphql: panic: %v\n%s", panicErr, buf)
		}
	}()
	return field.Resolve(ctx, source, args, selectionSet)
}

type objectCacheKey struct {
	typ          *Object
	source       interface{}
	selectionSet *SelectionSet
}

// executeObject executes an object query
func executeObject(ctx context.Context, typ *Object, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	// cache the body of executeObject so that if the source doesn't change, we
	// don't need to recompute
	key := objectCacheKey{typ: typ, source: source, selectionSet: selectionSet}

	// some types can't be put in a map; for those, use a always different value as source
	if !value.Type().Comparable() {
		key.source = &struct{}{}
	}

	return thunder.Cache(ctx, key, func(ctx context.Context) (interface{}, error) {
		selections := Flatten(selectionSet)

		fields := make(map[string]interface{})

		// for every selection, resolve the value and store it in the output object
		for _, selection := range selections {
			field := typ.Fields[selection.Name]
			value, err := safeResolve(ctx, field, source, selection.Args, selection.SelectionSet)
			if err != nil {
				return nil, err
			}

			resolved, err := execute(ctx, field.Type, value, selection.SelectionSet)
			if err != nil {
				return nil, err
			}
			fields[selection.Alias] = resolved
		}

		// if the source has a key, store it to detect changing objects
		var key interface{}
		if typ.Key != nil {
			value, err := typ.Key.Resolve(ctx, source, nil, nil)
			if err != nil {
				return nil, err
			}
			key = value
		}

		return &DiffableObject{Key: key, Fields: fields}, nil
	})
}

var emptyDiffableList = &DiffableList{Items: []interface{}{}}

// executeList executes a set query
func executeList(ctx context.Context, typ *List, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	if reflect.ValueOf(source).IsNil() {
		return emptyDiffableList, nil
	}

	// iterate over arbitrary slice types using reflect
	slice := reflect.ValueOf(source)
	items := make([]interface{}, slice.Len())

	// resolve every element in the slice
	if selectionSet != nil && selectionSet.Complex {
		var wg sync.WaitGroup
		var mu sync.Mutex
		var errs *multierror.Error

		for i := 0; i < slice.Len(); i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				thunder.AcquireGoroutineToken(ctx)
				defer thunder.ReleaseGoroutineToken(ctx)

				value := slice.Index(i)
				resolved, err := execute(ctx, typ.Type, value.Interface(), selectionSet)
				if err != nil {
					mu.Lock()
					errs = multierror.Append(errs, err)
					mu.Unlock()
					return
				}

				items[i] = resolved
			}(i)
		}

		thunder.ReleaseGoroutineToken(ctx)
		wg.Wait()
		thunder.AcquireGoroutineToken(ctx)

		if errs != nil {
			return nil, errs
		}

	} else {
		for i := 0; i < slice.Len(); i++ {
			value := slice.Index(i)
			resolved, err := execute(ctx, typ.Type, value.Interface(), selectionSet)
			if err != nil {
				return nil, err
			}
			items[i] = resolved
		}
	}

	return &DiffableList{Items: items}, nil
}

// execute executes a query by dispatches according to typ
func execute(ctx context.Context, typ Type, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch typ := typ.(type) {
	case *Scalar:
		return source, nil
	case *Object:
		return executeObject(ctx, typ, source, selectionSet)
	case *List:
		return executeList(ctx, typ, source, selectionSet)
	default:
		panic("unknown type kind")
	}
}

type Executor struct {
	MaxConcurrency int
}

// Execute executes a query by dispatches according to typ
func (e *Executor) Execute(ctx context.Context, typ Type, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	ctx = thunder.WithConcurrencyLimiter(ctx, e.MaxConcurrency)
	return execute(ctx, typ, source, selectionSet)
}
