package graphql

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"go.uber.org/atomic"
)

func NewQueue() *Queue {
	return &Queue{
		queue: make(chan *ExecutionUnit, 10000),
		done:  make(chan struct{}, 0),
	}
}

type Queue struct {
	mu sync.Mutex
	// TODO this can deadlock DANGER
	queue          chan *ExecutionUnit
	pendingCounter atomic.Int64
	done           chan struct{}
}

func (q *Queue) Enqueue(units ...*ExecutionUnit) {
	for _, unit := range units {
		q.pendingCounter.Inc()
		q.queue <- unit
	}
}

func (q *Queue) Dequeue() (*ExecutionUnit, func(), bool) {
	unit, ok := <-q.queue
	if !ok {
		return nil, nil, false
	}
	return unit, func() {
		q.pendingCounter.Dec()
		if q.pendingCounter.Load() == 0 {
			q.mu.Lock()
			defer q.mu.Unlock()

			if q.queue == nil {
				return
			}
			close(q.queue)
			q.queue = nil

			close(q.done)
		}
	}, ok
}

func (q *Queue) ClosedChan() chan struct{} {
	return q.done
}

type BatchExecutor struct {
	Queue []*ExecutionUnit
}

// Execute executes a query by dispatches according to typ
// It must return a JSON marshallable response.
func (e *BatchExecutor) Execute(ctx context.Context, typ Type, source interface{}, query *Query) (interface{}, error) {
	// TODO wrap ctx
	queryObject := typ.(*Object)
	selections := Flatten(query.SelectionSet)
	queue := make([]*ExecutionUnit, 0, 0)
	writers := make(map[string]OutputWriter)
	for _, selection := range selections {
		field, ok := queryObject.Fields[selection.Name]
		if !ok {
			return nil, fmt.Errorf("Invalid selection %q", selection.Name)
		}
		outputWriter := &ObjectWriter{}
		writers[selection.Alias] = outputWriter
		// TODO VALIDATE?
		queue = append(
			queue,
			&ExecutionUnit{
				Ctx:          ctx, // TODO add GQL execution path to ctx
				Sources:      []interface{}{source},
				Field:        field,
				Destinations: []OutputWriter{outputWriter},
				Selection:    selection,
			},
		)
	}

	execQueue := NewQueue()

	execQueue.Enqueue(queue...)

	for i := 0; i < 10000; i++ {
		// Lazy allocate goroutines (FF configurable?)
		go func() {
			for {
				ok := runEnqueue(execQueue)
				if !ok {
					return
				}
			}
		}()
	}
	// READ FROM INPUT QUEUE
	// RUN NEW EXECUTORS
	// IF NO RUNNING EXECUTORS AND NO QUEUE, EXIT

	<-execQueue.ClosedChan()
	// FIND ERROR?
	return writers, nil
}

func runEnqueue(execQueue *Queue) bool {
	// PANIC WRAP
	unit, done, ok := execQueue.Dequeue()
	if !ok {
		return ok
	}
	defer done()
	fmt.Println(unit, unit.Field, unit.Field.BatchResolve)
	fmt.Println(unit.Field.Type.String())
	units := unit.Field.BatchResolve(unit)
	execQueue.Enqueue(units...)
	return true
}

func UnwrapBatchResult(ctx context.Context, sources []interface{}, typ Type, selectionSet *SelectionSet, destinations []OutputWriter) ([]*ExecutionUnit, error) {
	// Ignore if context done
	switch typ := typ.(type) {
	case *Scalar:
		for i, source := range sources {
			if typ.Unwrapper == nil {
				destinations[i].Fill(unwrap(source))
				continue
			}
			res, err := typ.Unwrapper(source)
			if err != nil {
				return nil, err
			}
			destinations[i].Fill(res)
		}
		return nil, nil
	case *Enum:
		for i, source := range sources {
			val := unwrap(source)
			if mapVal, ok := typ.ReverseMap[val]; !ok {
				err := errors.New("enum is not valid")
				destinations[i].Fail(err)
				return nil, err
			} else {
				destinations[i].Fill(mapVal)
			}
		}
		return nil, nil
	//case *Union:
	//	return e.executeUnion(ctx, typ, source, selectionSet)
	case *Object:
		return UnwrapBatchObjectResult(ctx, sources, typ, selectionSet, destinations)
	case *List:
		flattenedResps := make([]OutputWriter, 0, len(sources))
		flattenedSources := make([]interface{}, 0, len(sources))
		for idx, source := range sources {
			slice := reflect.ValueOf(source)
			respList := make([]interface{}, slice.Len())
			for i := 0; i < slice.Len(); i++ {
				writer := NewObjectWriter(destinations[idx]) // TODO Parent?
				respList[i] = writer
				flattenedResps = append(flattenedResps, writer)
				flattenedSources = append(flattenedSources, slice.Index(i).Interface())
			}
			destinations[idx].Fill(respList)
		}
		return UnwrapBatchResult(ctx, flattenedSources, typ.Type, selectionSet, flattenedResps)
	case *NonNull:
		return UnwrapBatchResult(ctx, sources, typ.Type, selectionSet, destinations)
	default:
		panic(typ)
	}
}

//func UnwrapBatchUnionResult(ctx context.Context, sources []interface{}, typ *Union, selectionSet *SelectionSet, destinations []OutputWriter) ([]*ExecutionUnit, error) {
//
//}

func UnwrapBatchObjectResult(ctx context.Context, sources []interface{}, typ *Object, selectionSet *SelectionSet, destinations []OutputWriter) ([]*ExecutionUnit, error) {
	selections := Flatten(selectionSet)
	numExpensive := 0
	numNonExpensive := 0
	for _, selection := range selections {
		if field, ok := typ.Fields[selection.Name]; ok && field.Expensive {
			numExpensive++
		} else {
			numNonExpensive++
		}
	}

	nonNilSources := make([]interface{}, 0, len(sources))
	nonNilDestinations := make([]map[string]interface{}, 0, len(destinations))
	originDestinations := make([]OutputWriter, 0, len(destinations))
	for idx, source := range sources {
		value := reflect.ValueOf(source)
		if value.Kind() == reflect.Ptr && value.IsNil() {
			destinations[idx].Fill(nil)
			continue
		}
		nonNilSources = append(nonNilSources, source)
		destMap := make(map[string]interface{}, len(selections))
		destinations[idx].Fill(destMap)
		nonNilDestinations = append(nonNilDestinations, destMap)
		originDestinations = append(originDestinations, destinations[idx])
	}

	//fields := make(map[string]interface{})
	// Number of Execution Units = (NumExpensiveFields x NumSources) + NumNonExpensiveFields
	executionUnits := make([]*ExecutionUnit, 0, numNonExpensive+(numExpensive*len(nonNilSources)))

	// for every selection, resolve the value or schedule an execution unit for the field
	for _, selection := range selections {
		if selection.Name == "__typename" { // TODO make a fieldFunc?
			for idx := range nonNilDestinations {
				nonNilDestinations[idx][selection.Alias] = typ.Name
			}
			continue
		}

		destForSelection := make([]OutputWriter, 0, len(nonNilDestinations))
		for idx, destMap := range nonNilDestinations {
			filler := NewObjectWriter(originDestinations[idx])
			destForSelection = append(destForSelection, filler)
			destMap[selection.Alias] = filler
		}

		field := typ.Fields[selection.Name]
		if field.Batch {
			executionUnits = append(executionUnits, &ExecutionUnit{
				Ctx:          ctx,
				Field:        field,
				Sources:      nonNilSources,
				Destinations: destForSelection,
				Selection:    selection,
			})
			continue
		}
		if field.Expensive {
			// Multiple Units
			for idx, source := range nonNilSources {
				executionUnits = append(executionUnits, &ExecutionUnit{
					Ctx:          ctx,
					Field:        field,
					Sources:      []interface{}{source},
					Destinations: []OutputWriter{destForSelection[idx]},
					Selection:    selection,
				})
			}
			continue
		}
		if field.Unboundable { // FieldFunc but supposed to be "Fast" (I don't trust them)
			executionUnits = append(executionUnits, &ExecutionUnit{
				Ctx:          ctx,
				Field:        field,
				Sources:      nonNilSources,
				Destinations: destForSelection,
				Selection:    selection,
			})
			continue
		}
		// Resolve functions individually
		executionUnits = append(
			executionUnits,
			field.BatchResolve(&ExecutionUnit{
				Ctx:          ctx,
				Field:        field,
				Sources:      nonNilSources,
				Destinations: destForSelection,
				Selection:    selection,
			})...,
		)
	}

	if typ.Key != nil {
		destForSelection := make([]OutputWriter, 0, len(nonNilDestinations))
		for idx, destMap := range nonNilDestinations {
			filler := NewObjectWriter(originDestinations[idx])
			destForSelection = append(destForSelection, filler)
			destMap["__key"] = filler
		}
		executionUnits = append(
			executionUnits,
			typ.KeyField.BatchResolve(&ExecutionUnit{
				Ctx:          ctx,
				Field:        typ.KeyField,
				Sources:      nonNilSources,
				Destinations: destForSelection,
				Selection:    &Selection{},
			})...,
		)
	}

	return executionUnits, nil
}
