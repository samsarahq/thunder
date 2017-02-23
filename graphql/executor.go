package graphql

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sync"

	"github.com/samsarahq/thunder/reactive"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

type pathError struct {
	inner error
	path  []string
}

func nestPathError(key string, err error) error {
	// Don't nest SanitzedError's, as they are intended for human consumption.
	if se, ok := err.(SanitizedError); ok {
		return se
	}

	if pe, ok := err.(*pathError); ok {
		return &pathError{
			inner: pe.inner,
			path:  append(pe.path, key),
		}
	}

	return &pathError{
		inner: err,
		path:  []string{key},
	}
}

func extractPathError(err error) error {
	if pe, ok := err.(*pathError); ok {
		return pe.inner
	}
	return err
}

func (pe *pathError) Error() string {
	var buffer bytes.Buffer
	for i := len(pe.path) - 1; i >= 0; i-- {
		if i < len(pe.path)-1 {
			buffer.WriteString(".")
		}
		buffer.WriteString(pe.path[i])
	}
	buffer.WriteString(": ")
	buffer.WriteString(pe.inner.Error())
	return buffer.String()
}

func isNilArgs(args interface{}) bool {
	m, ok := args.(map[string]interface{})
	return args == nil || (ok && len(m) == 0)
}

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
			if selection.Name == "__typename" {
				if !isNilArgs(selection.Args) {
					return NewSafeError(`error parsing args for "__typename": no args expected`)
				}
				if selection.SelectionSet != nil {
					return NewSafeError(`scalar field "__typename" must have no selection`)
				}
				continue
			}

			field, ok := typ.Fields[selection.Name]
			if !ok {
				return NewSafeError(`unknown field "%s"`, selection.Name)
			}

			parsed, err := field.ParseArguments(selection.Args)
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

type resolveAndExecuteCacheKey struct {
	field     *Field
	source    interface{}
	selection *Selection
}

func (e *Executor) resolveAndExecute(ctx context.Context, field *Field, source interface{}, selection *Selection) (interface{}, error) {
	var span opentracing.Span
	if _, ok := field.Type.(*Scalar); !ok {
		span, ctx = opentracing.StartSpanFromContext(ctx, fmt.Sprintf("thunder.graphql.resolve(%s)", selection.Alias))
		span.LogFields(
			log.Bool("expensive", field.Expensive),
		)
	}

	if field.Expensive {
		// TODO: Skip goroutine for cached value
		return fork(func() (interface{}, error) {
			value := reflect.ValueOf(source)
			// cache the body of resolve and excecute so that if the source doesn't change, we
			// don't need to recompute
			key := resolveAndExecuteCacheKey{field: field, source: source, selection: selection}

			// some types can't be put in a map; for those, use a always different value
			// as source
			if value.IsValid() && !value.Type().Comparable() {
				// TODO: Warn, or somehow prevent using type-system?
				key.source = new(byte)
			}

			// TODO: Consider cacheing resolve and execute independently
			fromCache := true
			defer func() {
				if span != nil {
					span.LogFields(
						log.Bool("cached", fromCache),
					)
					span.Finish()
				}
			}()
			return reactive.Cache(ctx, key, func(ctx context.Context) (interface{}, error) {
				fromCache = false
				value, err := safeResolve(ctx, field, source, selection.Args, selection.SelectionSet)
				if err != nil {
					return nil, err
				}
				e.mu.Lock()
				value, err = e.execute(ctx, field.Type, value, selection.SelectionSet)
				e.mu.Unlock()

				if err != nil {
					return nil, err
				}
				return await(value)
			})
		}), nil
	}

	defer func() {
		if span != nil {
			span.Finish()
		}
	}()
	value, err := safeResolve(ctx, field, source, selection.Args, selection.SelectionSet)
	if err != nil {
		return nil, err
	}
	return e.execute(ctx, field.Type, value, selection.SelectionSet)
}

// executeObject executes an object query
func (e *Executor) executeObject(ctx context.Context, typ *Object, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	selections := Flatten(selectionSet)

	fields := make(map[string]interface{})

	// for every selection, resolve the value and store it in the output object
	for _, selection := range selections {
		if selection.Name == "__typename" {
			fields[selection.Alias] = typ.Name
			continue
		}

		field := typ.Fields[selection.Name]
		resolved, err := e.resolveAndExecute(ctx, field, source, selection)
		if err != nil {
			return nil, nestPathError(selection.Alias, err)
		}
		fields[selection.Alias] = resolved
	}

	var key interface{}
	if typ.Key != nil {
		value, err := e.resolveAndExecute(ctx, &Field{Type: &Scalar{Type: "string"}, Resolve: typ.Key}, source, &Selection{})
		if err != nil {
			return nil, nestPathError("__key", err)
		}
		key = value
	}

	return &awaitableDiffableObject{Fields: fields, Key: key}, nil
}

var emptyDiffableList = &DiffableList{Items: []interface{}{}}

// executeList executes a set query
func (e *Executor) executeList(ctx context.Context, typ *List, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	if reflect.ValueOf(source).IsNil() {
		return emptyDiffableList, nil
	}

	// iterate over arbitrary slice types using reflect
	slice := reflect.ValueOf(source)
	items := make([]interface{}, slice.Len())

	// resolve every element in the slice
	for i := 0; i < slice.Len(); i++ {
		value := slice.Index(i)
		resolved, err := e.execute(ctx, typ.Type, value.Interface(), selectionSet)
		if err != nil {
			return nil, nestPathError(fmt.Sprint(i), err)
		}
		items[i] = resolved
	}

	return &awaitableDiffableList{Items: items}, nil
}

// execute executes a query by dispatches according to typ
func (e *Executor) execute(ctx context.Context, typ Type, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch typ := typ.(type) {
	case *Scalar:
		return source, nil
	case *Object:
		return e.executeObject(ctx, typ, source, selectionSet)
	case *List:
		return e.executeList(ctx, typ, source, selectionSet)
	default:
		panic(typ)
	}
}

type Executor struct {
	mu sync.Mutex
}

// Execute executes a query by dispatches according to typ
func (e *Executor) Execute(ctx context.Context, typ Type, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	e.mu.Lock()
	value, err := e.execute(ctx, typ, source, selectionSet)
	e.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return await(value)
}
