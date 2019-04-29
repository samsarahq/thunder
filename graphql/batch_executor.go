package graphql

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/samsarahq/thunder/reactive"
)

// WorkUnit is a set of execution work that will be done when running
// a graphql query.  For every source there is an equivalent destination
// OutputNode that is used to record the result of running a section of the
// graphql query.
type WorkUnit struct {
	ctx          context.Context
	field        *Field
	selection    *Selection
	sources      []interface{}
	destinations []*outputNode
}

// UnitResolver is a function that executes a function and returns a set of
// new work units that need to be run.
type UnitResolver func(*WorkUnit) []*WorkUnit

// WorkScheduler is an interface that can be provided to the BatchExecutor
// to control how we traverse the Execution graph.  Examples would include using
// a bounded goroutine pool, or using unbounded goroutine generation for each
// work unit.
type WorkScheduler interface {
	Run(resolver UnitResolver, startingUnits ...*WorkUnit)
}

func NewBatchExecutor(scheduler WorkScheduler) *BatchExecutor {
	return &BatchExecutor{
		scheduler: scheduler,
	}
}

// BatchExecutor is a GraphQL executor.  Given a query it can run through the
// execution of the request.
type BatchExecutor struct {
	scheduler WorkScheduler
}

// Execute executes a query by traversing the GraphQL query graph and resolving
// or executing fields.  Any work that needs to be done is passed off to the
// scheduler to handle managing concurrency of the request.
// It must return a JSON marshallable response (or an error).
func (e *BatchExecutor) Execute(ctx context.Context, typ Type, source interface{}, query *Query) (interface{}, error) {
	queryObject, ok := typ.(*Object)
	if !ok {
		return nil, fmt.Errorf("expected query or mutation object for execution, got: %s", typ.String())
	}

	topLevelSelections := Flatten(query.SelectionSet)
	topLevelRespWriter := newTopLevelOutputNode(query.Name)
	initialSelectionWorkUnits := make([]*WorkUnit, 0, len(topLevelSelections))
	writers := make(map[string]*outputNode)
	for _, selection := range topLevelSelections {
		field, ok := queryObject.Fields[selection.Name]
		if !ok {
			return nil, fmt.Errorf("invalid top-level selection %q", selection.Name)
		}

		writer := newOutputNode(topLevelRespWriter, selection.Alias)
		writers[selection.Alias] = writer

		initialSelectionWorkUnits = append(
			initialSelectionWorkUnits,
			&WorkUnit{
				ctx:          ctx,
				sources:      []interface{}{source},
				field:        field,
				destinations: []*outputNode{writer},
				selection:    selection,
			},
		)
	}

	e.scheduler.Run(executeWorkUnit, initialSelectionWorkUnits...)

	if topLevelRespWriter.errRecorder.err != nil {
		return nil, topLevelRespWriter.errRecorder.err
	}
	return writers, nil
}

// executeWorkUnit executes/resolves a work unit and checks the
// selections of the unit to determine if it needs to schedule more work (which
// will be returned as new work units that will need to get scheduled.
func executeWorkUnit(unit *WorkUnit) []*WorkUnit {
	if unit.field.Batch && unit.selection.UseBatch {
		return executeBatchWorkUnit(unit)
	}

	var units []*WorkUnit
	for idx, src := range unit.sources {
		if !unit.field.Expensive {
			units = append(units, executeNonBatchWorkUnit(unit.ctx, src, unit.destinations[idx], unit)...)
			continue
		}
		units = append(units, executeNonBatchWorkUnitWithCaching(src, unit.destinations[idx], unit)...)
	}
	return units
}

func executeBatchWorkUnit(unit *WorkUnit) []*WorkUnit {
	results, err := safeExecuteBatchResolver(unit.ctx, unit.field, unit.sources, unit.selection.Args, unit.selection.SelectionSet)
	if err != nil {
		for _, dest := range unit.destinations {
			dest.Fail(err)
		}
		return nil
	}
	unitChildren, err := resolveBatch(unit.ctx, results, unit.field.Type, unit.selection.SelectionSet, unit.destinations)
	if err != nil {
		for _, dest := range unit.destinations {
			dest.Fail(err)
		}
		return nil
	}
	return unitChildren
}

// executeNonBatchWorkUnitWithCaching wraps a resolve request in a reactive cache
// call.
// This function makes two assumptions:
// - We assume that all the reactive cache will get cleared if there is an error.
// - We assume that there is no "error-catching" mechanism that will stop an
//   error from propagating all the way to the top of the request stack.
func executeNonBatchWorkUnitWithCaching(src interface{}, dest *outputNode, unit *WorkUnit) []*WorkUnit {
	var workUnits []*WorkUnit
	subDestRes, err := reactive.Cache(unit.ctx, getWorkCacheKey(src, unit.field, unit.selection), func(ctx context.Context) (interface{}, error) {
		subDest := newOutputNode(dest, "")
		workUnits = executeNonBatchWorkUnit(ctx, src, subDest, unit)
		return subDest.res, nil
	})
	if err != nil {
		dest.Fail(err)
	}
	dest.Fill(subDestRes)
	return workUnits
}

// getWorkCacheKey gets the work cache key for the provided source.
func getWorkCacheKey(src interface{}, field *Field, selection *Selection) resolveAndExecuteCacheKey {
	value := reflect.ValueOf(src)
	// cache the body of resolve and execute so that if the source doesn't change, we
	// don't need to recompute
	key := resolveAndExecuteCacheKey{field: field, source: src, selection: selection}
	// some types can't be put in a map; for those, use a always different value
	// as source
	if value.IsValid() && !value.Type().Comparable() {
		// TODO: Warn, or somehow prevent using type-system?
		key.source = new(byte)
	}
	return key
}

// executeNonBatchWorkUnit resolves a non-batch field in our graphql response graph.
func executeNonBatchWorkUnit(ctx context.Context, src interface{}, dest *outputNode, unit *WorkUnit) []*WorkUnit {
	fieldResult, err := safeExecuteResolver(ctx, unit.field, src, unit.selection.Args, unit.selection.SelectionSet)
	if err != nil {
		dest.Fail(err)
		return nil
	}
	subFieldWorkUnits, err := resolveBatch(ctx, []interface{}{fieldResult}, unit.field.Type, unit.selection.SelectionSet, []*outputNode{dest})
	if err != nil {
		dest.Fail(err)
		return nil
	}
	return subFieldWorkUnits
}

// resolveBatch traverses the provided sources and fills in result data and
// returns new work units that are required to resolve the rest of the
// query result.
func resolveBatch(ctx context.Context, sources []interface{}, typ Type, selectionSet *SelectionSet, destinations []*outputNode) ([]*WorkUnit, error) {
	switch typ := typ.(type) {
	case *Scalar:
		return nil, resolveScalarBatch(sources, typ, destinations)
	case *Enum:
		return nil, resolveEnumBatch(sources, typ, destinations)
	case *List:
		return resolveListBatch(ctx, sources, typ, selectionSet, destinations)
	case *Union:
		return resolveUnionBatch(ctx, sources, typ, selectionSet, destinations)
	case *Object:
		return resolveObjectBatch(ctx, sources, typ, selectionSet, destinations)
	case *NonNull:
		return resolveBatch(ctx, sources, typ.Type, selectionSet, destinations)
	default:
		panic(typ)
	}
}

// Resolves the scalar type value for all the provided sources.
func resolveScalarBatch(sources []interface{}, typ *Scalar, destinations []*outputNode) error {
	for i, source := range sources {
		if typ.Unwrapper == nil {
			destinations[i].Fill(unwrap(source))
			continue
		}
		res, err := typ.Unwrapper(source)
		if err != nil {
			return err
		}
		destinations[i].Fill(res)
	}
	return nil
}

// Resolves the enum type value for all the provided sources.
func resolveEnumBatch(sources []interface{}, typ *Enum, destinations []*outputNode) error {
	for i, source := range sources {
		val := unwrap(source)
		if mapVal, ok := typ.ReverseMap[val]; !ok {
			err := errors.New("enum is not valid")
			destinations[i].Fail(err)
			return err
		} else {
			destinations[i].Fill(mapVal)
		}
	}
	return nil
}

// Flattens the sources for the list type and calls into an unwrapper method for
// the list's subtype.
func resolveListBatch(ctx context.Context, sources []interface{}, typ *List, selectionSet *SelectionSet, destinations []*outputNode) ([]*WorkUnit, error) {
	reflectedSources := make([]reflect.Value, len(sources))
	numFlattenedSources := 0
	for idx, source := range sources {
		reflectedSources[idx] = reflect.ValueOf(source)
		numFlattenedSources += reflectedSources[idx].Len()
	}

	flattenedResps := make([]*outputNode, 0, numFlattenedSources)
	flattenedSources := make([]interface{}, 0, numFlattenedSources)
	for idx, slice := range reflectedSources {
		respList := make([]interface{}, slice.Len())
		for i := 0; i < slice.Len(); i++ {
			writer := newOutputNode(destinations[idx], strconv.Itoa(idx))
			respList[i] = writer
			flattenedResps = append(flattenedResps, writer)
			flattenedSources = append(flattenedSources, slice.Index(i).Interface())
		}
		destinations[idx].Fill(respList)
	}
	return resolveBatch(ctx, flattenedSources, typ.Type, selectionSet, flattenedResps)
}

// Traverses the Union type and resolves or creates work units to resolve
// all of the sub-objects for all the provided sources.
func resolveUnionBatch(ctx context.Context, sources []interface{}, typ *Union, selectionSet *SelectionSet, destinations []*outputNode) ([]*WorkUnit, error) {
	sourcesByType := make(map[string][]interface{}, len(typ.Types))
	destinationsByType := make(map[string][]*outputNode, len(typ.Types))
	for idx, src := range sources {
		union := reflect.ValueOf(src)
		if union.Kind() == reflect.Ptr && union.IsNil() {
			// Don't create a destination for any nil Unions types
			destinations[idx].Fill(nil)
			continue
		}

		srcType := ""
		if union.Kind() == reflect.Ptr && union.Elem().Kind() == reflect.Struct {
			union = union.Elem()
		}
		for typString := range typ.Types {
			inner := union.FieldByName(typString)
			if inner.IsNil() {
				continue
			}
			if srcType != "" {
				return nil, fmt.Errorf("union type field should only return one value, but received: %s %s", srcType, typString)
			}
			srcType = typString
			sourcesByType[srcType] = append(sourcesByType[srcType], inner.Interface())
			destinationsByType[srcType] = append(destinationsByType[srcType], destinations[idx])
		}
	}

	var workUnits []*WorkUnit
	for srcType, sources := range sourcesByType {
		gqlType := typ.Types[srcType]
		for _, fragment := range selectionSet.Fragments {
			if fragment.On != srcType {
				continue
			}
			units, err := resolveObjectBatch(ctx, sources, gqlType, fragment.SelectionSet, destinationsByType[srcType])
			if err != nil {
				return nil, err
			}
			workUnits = append(workUnits, units...)
		}

	}
	return workUnits, nil
}

// Traverses the object selections and resolves or creates work units to resolve
// all of the object fields for every source passed in.
func resolveObjectBatch(ctx context.Context, sources []interface{}, typ *Object, selectionSet *SelectionSet, destinations []*outputNode) ([]*WorkUnit, error) {
	selections := Flatten(selectionSet)
	numExpensive := 0
	numNonExpensive := 0
	for _, selection := range selections {
		if field, ok := typ.Fields[selection.Name]; ok && field.Expensive {
			numExpensive++
		} else if ok && field.External {
			numNonExpensive++
		}
	}

	// For every object, create a "destination" map that we can fill with our
	// result values.  Filter out invalid/nil objects.
	nonNilSources := make([]interface{}, 0, len(sources))
	nonNilDestinations := make([]map[string]interface{}, 0, len(destinations))
	originDestinations := make([]*outputNode, 0, len(destinations))
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

	// Number of Work Units = (NumExpensiveFields x NumSources) + NumNonExpensiveFields
	workUnits := make([]*WorkUnit, 0, numNonExpensive+(numExpensive*len(nonNilSources)))

	// for every selection, resolve the value or schedule an work unit for the field
	for _, selection := range selections {
		if selection.Name == "__typename" {
			for idx := range nonNilDestinations {
				nonNilDestinations[idx][selection.Alias] = typ.Name
			}
			continue
		}

		destForSelection := make([]*outputNode, 0, len(nonNilDestinations))
		for idx, destMap := range nonNilDestinations {
			filler := newOutputNode(originDestinations[idx], selection.Alias)
			destForSelection = append(destForSelection, filler)
			destMap[selection.Alias] = filler
		}

		field := typ.Fields[selection.Name]
		if field.Batch && selection.UseBatch {
			workUnits = append(workUnits, &WorkUnit{
				ctx:          ctx,
				field:        field,
				sources:      nonNilSources,
				destinations: destForSelection,
				selection:    selection,
			})
			continue
		}
		if field.Expensive {
			// Expensive fields should be executed as multiple "Units".  The scheduler
			// controls how the units are executed
			for idx, source := range nonNilSources {
				workUnits = append(workUnits, &WorkUnit{
					ctx:          ctx,
					field:        field,
					sources:      []interface{}{source},
					destinations: []*outputNode{destForSelection[idx]},
					selection:    selection,
				})
			}
			continue
		}
		unit := &WorkUnit{
			ctx:          ctx,
			field:        field,
			sources:      nonNilSources,
			destinations: destForSelection,
			selection:    selection,
		}
		if field.External {
			// External non-Expensive fields should be fast (so we can run them at the
			// same time), but, since they are still external functions we don't want
			// to run them where they could potentially block.
			// So we create an work unit with all the fields to execute
			// asynchronously.
			workUnits = append(workUnits, unit)
			continue
		}
		// If the fields are not expensive or external the work time should be
		// bounded, so we can resolve them immediately.
		workUnits = append(
			workUnits,
			executeWorkUnit(unit)...,
		)
	}

	if typ.KeyField != nil {
		destForSelection := make([]*outputNode, 0, len(nonNilDestinations))
		for idx, destMap := range nonNilDestinations {
			filler := newOutputNode(originDestinations[idx], "__key")
			destForSelection = append(destForSelection, filler)
			destMap["__key"] = filler
		}
		workUnits = append(
			workUnits,
			executeWorkUnit(&WorkUnit{
				ctx:          ctx,
				field:        typ.KeyField,
				sources:      nonNilSources,
				destinations: destForSelection,
				selection:    &Selection{},
			})...,
		)
	}

	return workUnits, nil
}
