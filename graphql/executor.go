package graphql

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/samsarahq/thunder/concurrencylimiter"
	"github.com/samsarahq/thunder/reactive"
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

func ErrorCause(err error) error {
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

// unwrap will return the value associated with a pointer type, or nil if the
// pointer is nil
func unwrap(v interface{}) interface{} {
	i := reflect.ValueOf(v)
	for i.Kind() == reflect.Ptr && !i.IsNil() {
		i = i.Elem()
	}
	if i.Kind() == reflect.Invalid {
		return nil
	}
	return i.Interface()
}

// PrepareQuery checks that the given selectionSet matches the schema typ, and
// parses the args in selectionSet
func PrepareQuery(typ Type, selectionSet *SelectionSet) error {
	switch typ := typ.(type) {
	case *Scalar:
		if selectionSet != nil {
			return NewClientError("scalar field must have no selections")
		}
		return nil
	case *Enum:
		if selectionSet != nil {
			return NewClientError("enum field must have no selections")
		}
		return nil
	case *Union:
		if selectionSet == nil {
			return NewClientError("object field must have selections")
		}

		for _, fragment := range selectionSet.Fragments {
			for typString, graphqlTyp := range typ.Types {
				if fragment.On != typString {
					continue
				}
				if err := PrepareQuery(graphqlTyp, fragment.SelectionSet); err != nil {
					return err
				}
			}
		}
		for _, selection := range selectionSet.Selections {
			if selection.Name == "__typename" {
				if !isNilArgs(selection.Args) {
					return NewClientError(`error parsing args for "__typename": no args expected`)
				}
				if selection.SelectionSet != nil {
					return NewClientError(`scalar field "__typename" must have no selection`)
				}
				continue
			}
			return NewClientError(`unknown field "%s"`, selection.Name)
		}
		return nil
	case *Object:
		if selectionSet == nil {
			return NewClientError("object field must have selections")
		}
		for _, selection := range selectionSet.Selections {
			if selection.Name == "__typename" {
				if !isNilArgs(selection.Args) {
					return NewClientError(`error parsing args for "__typename": no args expected`)
				}
				if selection.SelectionSet != nil {
					return NewClientError(`scalar field "__typename" must have no selection`)
				}
				continue
			}

			field, ok := typ.Fields[selection.Name]
			if !ok {
				return NewClientError(`unknown field "%s"`, selection.Name)
			}

			// Only parse args once for a given selection.
			if !selection.parsed {
				parsed, err := field.ParseArguments(selection.Args)
				if err != nil {
					return NewClientError(`error parsing args for "%s": %s`, selection.Name, err)
				}
				selection.Args = parsed
				selection.parsed = true
			}

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

	case *NonNull:
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
	if field.Expensive {
		// TODO: Skip goroutine for cached value
		ctx, release := concurrencylimiter.Acquire(ctx)
		return fork(func() (interface{}, error) {
			defer release()

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
			resolvedValue, err := reactive.Cache(ctx, key, func(ctx context.Context) (interface{}, error) {
				value, err := safeResolve(ctx, field, source, selection.Args, selection.SelectionSet)
				if err != nil {
					return nil, err
				}

				// Release concurrency token before recursing into execute. It will attempt to
				// grab another concurrency token.
				release()

				e.mu.Lock()
				value, err = e.execute(ctx, field.Type, value, selection.SelectionSet)
				e.mu.Unlock()

				if err != nil {
					return nil, err
				}
				return await(value)
			})

			return resolvedValue, err
		}), nil
	}

	value, err := safeResolve(ctx, field, source, selection.Args, selection.SelectionSet)
	if err != nil {
		return nil, err
	}
	return e.execute(ctx, field.Type, value, selection.SelectionSet)
}

func (e *Executor) executeUnion(ctx context.Context, typ *Union, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil, nil
	}

	fields := make(map[string]interface{})
	for _, selection := range selectionSet.Selections {
		if selection.Name == "__typename" {
			fields[selection.Alias] = typ.Name
			continue
		}
	}

	// For every inline fragment spread, check if the current concrete type
	// matches and execute that object.
	var possibleTypes []string
	for typString, graphqlTyp := range typ.Types {
		inner := reflect.ValueOf(source)
		if inner.Kind() == reflect.Ptr && inner.Elem().Kind() == reflect.Struct {
			inner = inner.Elem()
		}

		inner = inner.FieldByName(typString)
		if inner.IsNil() {
			continue
		}
		possibleTypes = append(possibleTypes, graphqlTyp.String())

		for _, fragment := range selectionSet.Fragments {
			if fragment.On != typString {
				continue
			}
			resolved, err := e.executeObject(ctx, graphqlTyp, inner.Interface(), fragment.SelectionSet)
			if err != nil {
				return nil, nestPathError(typString, err)
			}

			for k, v := range resolved.(map[string]interface{}) {
				fields[k] = v
			}
		}
	}

	if len(possibleTypes) > 1 {
		return nil, fmt.Errorf("union type field should only return one value, but received: %s", strings.Join(possibleTypes, " "))
	}
	return fields, nil
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

	if typ.Key != nil {
		value, err := e.resolveAndExecute(ctx, &Field{Type: &Scalar{Type: "string"}, Resolve: typ.Key}, source, &Selection{})
		if err != nil {
			return nil, nestPathError("__key", err)
		}
		fields["__key"] = value
	}

	return fields, nil
}

var emptyList = []interface{}{}

// executeList executes a set query
func (e *Executor) executeList(ctx context.Context, typ *List, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	if reflect.ValueOf(source).IsNil() {
		return emptyList, nil
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

	return items, nil
}

// execute executes a query by dispatches according to typ
func (e *Executor) execute(ctx context.Context, typ Type, source interface{}, selectionSet *SelectionSet) (interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch typ := typ.(type) {
	case *Scalar:
		if typ.Unwrapper != nil {
			return typ.Unwrapper(source)
		}
		return unwrap(source), nil
	case *Enum:
		val := unwrap(source)
		if mapVal, ok := typ.ReverseMap[val]; ok {
			return mapVal, nil
		}
		return nil, errors.New("enum is not valid")
	case *Union:
		return e.executeUnion(ctx, typ, source, selectionSet)
	case *Object:
		return e.executeObject(ctx, typ, source, selectionSet)
	case *List:
		return e.executeList(ctx, typ, source, selectionSet)
	case *NonNull:
		return e.execute(ctx, typ.Type, source, selectionSet)
	default:
		panic(typ)
	}
}

type Executor struct {
	mu sync.Mutex
}

// Execute executes a query by dispatches according to typ
func (e *Executor) Execute(ctx context.Context, typ Type, source interface{}, query *Query) (interface{}, error) {
	e.mu.Lock()
	value, err := e.execute(ctx, typ, source, query.SelectionSet)
	e.mu.Unlock()

	// Await the promise if things look good so far.
	if err == nil {
		value, err = await(value)
	}

	// Maybe error wrap if we have an error and a name to attach.
	if err != nil && query.Name != "" {
		err = nestPathError(query.Name, err)
	}

	return value, err
}
