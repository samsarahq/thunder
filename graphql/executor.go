package graphql

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"runtime"
)

type pathError struct {
	inner error
	path  []string
}

func nestPathErrorMulti(path []string, err error) error {
	// Don't nest SanitzedError's, as they are intended for human consumption.
	if se, ok := err.(SanitizedError); ok {
		return se
	}

	if pe, ok := err.(*pathError); ok {
		return &pathError{
			inner: pe.inner,
			path:  append(pe.path, path...),
		}
	}

	return &pathError{
		inner: err,
		path:  path,
	}
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

func (pe *pathError) Unwrap() error {
	return pe.inner
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

func isNilArgs(args map[string]interface{}) bool {
	return len(args) == 0
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
func PrepareQuery(typ Type, selectionSet *RawSelectionSet) (*SelectionSet, error) {
	return prepareQuery(typ, selectionSet, make(map[prepareQueryMemoKey]*SelectionSet))
}

type prepareQueryMemoKey struct {
	typ          Type
	selectionSet *RawSelectionSet
}

func prepareQuery(typ Type, selectionSet *RawSelectionSet, memo map[prepareQueryMemoKey]*SelectionSet) (res *SelectionSet, err error) {
	if res, ok := memo[prepareQueryMemoKey{typ: typ, selectionSet: selectionSet}]; ok {
		return res, nil
	}
	defer func() {
		if err == nil {
			memo[prepareQueryMemoKey{typ: typ, selectionSet: selectionSet}] = res
		}
	}()

	switch typ := typ.(type) {
	case *Scalar:
		if selectionSet != nil {
			return nil, NewClientError("scalar field must have no selections")
		}
		return nil, nil
	case *Enum:
		if selectionSet != nil {
			return nil, NewClientError("enum field must have no selections")
		}
		return nil, nil
	case *Union:
		if selectionSet == nil {
			return nil, NewClientError("object field must have selections")
		}

		newSelectionSet := &SelectionSet{}

		for _, fragment := range selectionSet.Fragments {
			for typString, graphqlTyp := range typ.Types {
				if fragment.On != typString {
					continue
				}
				newFragmentSelectionSet, err := prepareQuery(graphqlTyp, fragment.SelectionSet, memo)
				if err != nil {
					return nil, err
				}
				newSelectionSet.Fragments = append(newSelectionSet.Fragments, &Fragment{
					On:           fragment.On,
					SelectionSet: newFragmentSelectionSet,
				})
			}
		}
		for _, selection := range selectionSet.Selections {
			if selection.Name == "__typename" {
				if !isNilArgs(selection.Args) {
					return nil, NewClientError(`error parsing args for "__typename": no args expected`)
				}
				if selection.SelectionSet != nil {
					return nil, NewClientError(`scalar field "__typename" must have no selection`)
				}
				for _, fragment := range newSelectionSet.Fragments {
					fragment.SelectionSet.Selections = append(fragment.SelectionSet.Selections, &Selection{
						Alias: selection.Alias,
						Name:  "__typename",
					})
				}
				continue
			}
			return nil, NewClientError(`unknown field "%s"`, selection.Name)
		}
		return newSelectionSet, nil

	case *Object:
		if selectionSet == nil {
			return nil, NewClientError("object field must have selections")
		}

		newSelectionSet := &SelectionSet{}

		for _, selection := range selectionSet.Selections {
			if selection.Name == "__typename" {
				if !isNilArgs(selection.Args) {
					return nil, NewClientError(`error parsing args for "__typename": no args expected`)
				}
				if selection.SelectionSet != nil {
					return nil, NewClientError(`scalar field "__typename" must have no selection`)
				}
				newSelectionSet.Selections = append(newSelectionSet.Selections, &Selection{
					Alias: selection.Alias,
					Name:  "__typename",
				})
				continue
			}

			field, ok := typ.Fields[selection.Name]
			if !ok {
				return nil, NewClientError(`unknown field "%s"`, selection.Name)
			}

			parsed, err := field.ParseArguments(selection.Args)
			if err != nil {
				return nil, NewClientError(`error parsing args for "%s": %s`, selection.Name, err)
			}

			newChildSelectionSet, err := prepareQuery(field.Type, selection.SelectionSet, memo)
			if err != nil {
				return nil, err
			}

			newSelectionSet.Selections = append(newSelectionSet.Selections, &Selection{
				Alias:        selection.Alias,
				Name:         selection.Name,
				Args:         parsed,
				Flags:        selection.Flags,
				SelectionSet: newChildSelectionSet,
			})
		}
		for _, fragment := range selectionSet.Fragments {
			newFragmentSelectionSet, err := prepareQuery(typ, fragment.SelectionSet, memo)
			if err != nil {
				return nil, err
			}
			newSelectionSet.Fragments = append(newSelectionSet.Fragments, &Fragment{
				On:           fragment.On,
				SelectionSet: newFragmentSelectionSet,
			})
		}

		return newSelectionSet, nil

	case *List:
		return prepareQuery(typ.Type, selectionSet, memo)

	case *NonNull:
		return prepareQuery(typ.Type, selectionSet, memo)

	default:
		panic("unknown type kind")
	}
}

func SafeExecuteBatchResolver(ctx context.Context, field *Field, sources []interface{}, args interface{}, selectionSet *SelectionSet) (results []interface{}, err error) {
	defer func() {
		if panicErr := recover(); panicErr != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			results, err = nil, fmt.Errorf("graphql: panic: %v\n%s", panicErr, buf)
		}
	}()
	return field.BatchResolver(ctx, sources, args, selectionSet)
}

func SafeExecuteResolver(ctx context.Context, field *Field, source, args interface{}, selectionSet *SelectionSet) (result interface{}, err error) {
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

type ExecutorRunner interface {
	Execute(ctx context.Context, typ Type, query *Query) (interface{}, error)
}

type resolveAndExecuteCacheKey struct {
	field     *Field
	source    interface{}
	selection *Selection
}
