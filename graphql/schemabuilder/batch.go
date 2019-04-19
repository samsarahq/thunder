package schemabuilder

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql"
)

// buildBatchFunction corresponds to buildFunction for a batchFieldFunc
func (sb *schemaBuilder) buildBatchFunction(typ reflect.Type, m *method) (*graphql.Field, error) {
	funcCtx := &batchFuncContext{parentTyp: typ}

	if typ.Kind() == reflect.Ptr {
		return nil, fmt.Errorf("source-type of buildBatchFunction cannot be a pointer (got: %v)", typ)
	}

	callableFunc, err := funcCtx.getFuncVal(m)
	if err != nil {
		return nil, err
	}

	in := funcCtx.getFuncInputTypes()
	if len(in) == 0 {
		return nil, fmt.Errorf("batch Field funcs require at least one input field")
	}

	in = funcCtx.consumeContext(in)
	in, err = funcCtx.consumeRequiredSourceBatch(in)
	if err != nil {
		return nil, err
	}
	argParser, args, in, err := funcCtx.consumeArgs(sb, in)
	if err != nil {
		return nil, err
	}
	in = funcCtx.consumeSelectionSet(in)

	// We have succeeded if no arguments remain.
	if len(in) != 0 {
		return nil, fmt.Errorf("%s arguments should be [context,]map[int][*]%s[, args][, selectionSet]", funcCtx.funcType, typ)
	}

	out := funcCtx.getFuncOutputTypes()
	retType, out, err := funcCtx.consumeReturnValue(m, sb, out)
	if err != nil {
		return nil, err
	}
	out = funcCtx.consumeReturnError(out)
	if len(out) > 0 {
		return nil, fmt.Errorf("%s return should be [map[int]<Type>][,error]", funcCtx.funcType)
	}

	batchExecFunc := func(ctx context.Context, sources []interface{}, funcRawArgs interface{}, selectionSet *graphql.SelectionSet) ([]interface{}, error) {
		// Set up function arguments.
		funcInputArgs := funcCtx.prepareResolveArgs(sources, funcRawArgs, ctx, selectionSet)

		// Call the function.
		funcOutputArgs := callableFunc.Call(funcInputArgs)

		return funcCtx.extractResultsAndErr(len(sources), funcOutputArgs, retType)
	}

	batchFunc := batch.Func{
		Many: func(ctx context.Context, args []interface{}) ([]interface{}, error) {
			if len(args) == 0 {
				return nil, nil
			}
			funcRawArgs := args[0].(batchTypeHolder).funcRawArgs
			set := args[0].(batchTypeHolder).selectionSet

			sources := make([]interface{}, 0, len(args))
			for _, arg := range args {
				sources = append(sources, arg.(batchTypeHolder).source)
			}

			return batchExecFunc(ctx, sources, funcRawArgs, set)
		},
		MaxSize:      100,
		WaitInterval: time.Millisecond * 20,
	}

	return &graphql.Field{
		Resolve: func(ctx context.Context, source, funcRawArgs interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			// TODO do real batching
			return batchFunc.Invoke(ctx, batchTypeHolder{
				source:       source,
				funcRawArgs:  funcRawArgs,
				selectionSet: selectionSet,
			})
		},
		BatchResolveFunc: func(unit *graphql.ExecutionUnit) []*graphql.ExecutionUnit {
			panic("unimplemented")
		},
		Args:           args,
		Type:           retType,
		ParseArguments: argParser.Parse,
		Expensive:      funcCtx.hasContext,
	}, nil
}

type batchTypeHolder struct {
	source       interface{}
	funcRawArgs  interface{}
	selectionSet *graphql.SelectionSet
}

// funcContext is used to parse the function signature in buildFunction.
type batchFuncContext struct {
	hasContext      bool
	hasSource       bool
	hasArgs         bool
	hasSelectionSet bool
	hasRet          bool
	hasError        bool

	funcType     reflect.Type
	batchMapType reflect.Type
	isPtrFunc    bool
	parentTyp    reflect.Type
}

// getFuncVal returns a reflect.Value of an executable function.
func (funcCtx *batchFuncContext) getFuncVal(m *method) (reflect.Value, error) {
	fun := reflect.ValueOf(m.Fn)
	if fun.Kind() != reflect.Func {
		return fun, fmt.Errorf("fun must be func, not %s", fun)
	}
	funcCtx.funcType = fun.Type()
	return fun, nil
}

// getFuncInputTypes returns the input arguments for the function we're
// representing.
func (funcCtx *batchFuncContext) getFuncInputTypes() []reflect.Type {
	in := make([]reflect.Type, 0, funcCtx.funcType.NumIn())
	for i := 0; i < funcCtx.funcType.NumIn(); i++ {
		in = append(in, funcCtx.funcType.In(i))
	}
	return in
}

// consumeContext reads in the input parameters for the provided
// function and determines whether the function has a Context input parameter.
// It returns the input types without the context parameter if it was there.
func (funcCtx *batchFuncContext) consumeContext(in []reflect.Type) []reflect.Type {
	if len(in) > 0 && in[0] == contextType {
		funcCtx.hasContext = true
		in = in[1:]
	}
	return in
}

// consumeRequiredSourceBatch reads in the input parameters for the provided
// function and guarantees that the input parameters include a batch of the
// parent type (map[int]*ParentObject).  If we don't have the batch we return an
// error because the function is invalid.
func (funcCtx *batchFuncContext) consumeRequiredSourceBatch(in []reflect.Type) ([]reflect.Type, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("requires batch source input parameter for func")
	}
	inType := in[0]
	in = in[1:]

	parentPtrType := reflect.PtrTo(funcCtx.parentTyp)
	if inType.Kind() != reflect.Map ||
		inType.Key().Kind() != reflect.Int ||
		(inType.Elem() != parentPtrType && inType.Elem() != funcCtx.parentTyp) {
		return nil, fmt.Errorf(
			"invalid source batch type, expected one of map[int]*%s or map[int]%s, but got %s",
			funcCtx.parentTyp.String(),
			funcCtx.parentTyp.String(),
			inType.String(),
		)
	}

	funcCtx.isPtrFunc = inType.Elem() == parentPtrType
	funcCtx.batchMapType = inType

	return in, nil
}

// consumeArgs reads the args parameter if it is there and returns an argParser,
// argTypeMap and the filtered input parameters.
func (funcCtx *batchFuncContext) consumeArgs(sb *schemaBuilder, in []reflect.Type) (*argParser, map[string]graphql.Type, []reflect.Type, error) {
	if len(in) == 0 || in[0] == selectionSetType {
		return nil, nil, in, nil
	}
	inType := in[0]
	in = in[1:]
	argParser, argType, err := sb.makeStructParser(inType)
	if err != nil {
		return nil, nil, in, fmt.Errorf("attempted to parse %s as arguments struct, but failed: %s", inType.Name(), err.Error())
	}
	inputObject, ok := argType.(*graphql.InputObject)
	if !ok {
		return nil, nil, nil, fmt.Errorf("%s's args should be an object", funcCtx.funcType)
	}
	args := make(map[string]graphql.Type, len(inputObject.InputFields))
	for name, typ := range inputObject.InputFields {
		args[name] = typ
	}
	funcCtx.hasArgs = true
	return argParser, args, in, nil
}

// consumeSelectionSet reads the input parameters and will pop off the
// selectionSet type if we detect it in the input fields.  Check out
// graphql.SelectionSet for more infomation about selection sets.
func (funcCtx *batchFuncContext) consumeSelectionSet(in []reflect.Type) []reflect.Type {
	if len(in) > 0 && in[0] == selectionSetType {
		in = in[1:]
		funcCtx.hasSelectionSet = true
	}
	return in
}

func (funcCtx *batchFuncContext) getFuncOutputTypes() []reflect.Type {
	out := make([]reflect.Type, 0, funcCtx.funcType.NumOut())
	for i := 0; i < funcCtx.funcType.NumOut(); i++ {
		out = append(out, funcCtx.funcType.Out(i))
	}
	return out
}

// consumeReturnValue consumes the function output's response value if it exists
// and validates that the response is a proper batch type.
func (funcCtx *batchFuncContext) consumeReturnValue(m *method, sb *schemaBuilder, out []reflect.Type) (graphql.Type, []reflect.Type, error) {
	if len(out) == 0 || out[0] == errType {
		if m.MarkedNonNullable {
			return nil, nil, fmt.Errorf("%s is marked non-nullable, but has no return value", funcCtx.funcType)
		}
		retType, err := sb.getType(reflect.TypeOf(true))
		if err != nil {
			return nil, nil, err
		}
		return retType, out, nil
	}
	outType := out[0]
	out = out[1:]
	if outType.Kind() != reflect.Map ||
		outType.Key().Kind() != reflect.Int {
		return nil, nil, fmt.Errorf(
			"invalid response batch type, expected map[int]<Type>, but got %s",
			outType.String(),
		)
	}
	retType, err := sb.getType(outType.Elem())
	if err != nil {
		return nil, nil, err
	}
	if m.MarkedNonNullable { // TODO see if we can even enforce non-nil resps for batch endpoints
		if _, ok := retType.(*graphql.NonNull); !ok {
			retType = &graphql.NonNull{Type: retType}
		}
	}
	funcCtx.hasRet = true
	return retType, out, nil
}

// consumeReturnValue consumes the function output's error type if it exists.
func (funcCtx *batchFuncContext) consumeReturnError(out []reflect.Type) []reflect.Type {
	if len(out) > 0 && out[0] == errType {
		funcCtx.hasError = true
		out = out[1:]
	}
	return out
}

// prepareResolveArgs converts the provided source, args and context into the
// required list of reflect.Value types that the function needs to be called.
func (funcCtx *batchFuncContext) prepareResolveArgs(sources []interface{}, args interface{}, ctx context.Context, selectionSet *graphql.SelectionSet) []reflect.Value {
	in := make([]reflect.Value, 0, funcCtx.funcType.NumIn())
	if funcCtx.hasContext {
		in = append(in, reflect.ValueOf(ctx))
	}

	batch := reflect.MakeMapWithSize(funcCtx.batchMapType, len(sources))
	for idx, source := range sources {
		idxVal := idx
		sourceValue := reflect.ValueOf(source)
		ptrSource := sourceValue.Kind() == reflect.Ptr
		switch {
		case ptrSource && !funcCtx.isPtrFunc:
			batch.SetMapIndex(reflect.ValueOf(idxVal), sourceValue.Elem())
		case !ptrSource && funcCtx.isPtrFunc:
			copyPtr := reflect.New(funcCtx.parentTyp)
			copyPtr.Elem().Set(sourceValue)
			batch.SetMapIndex(reflect.ValueOf(idxVal), copyPtr)
		default:
			batch.SetMapIndex(reflect.ValueOf(idxVal), sourceValue)
		}
	}
	in = append(in, batch)

	// Set up other arguments.
	if funcCtx.hasArgs {
		in = append(in, reflect.ValueOf(args))
	}
	if funcCtx.hasSelectionSet {
		in = append(in, reflect.ValueOf(selectionSet))
	}

	return in
}

// extractResultsAndErr converts the response from calling the function into
// the expected type for the response object (as opposed to a reflect.Value).
// It also handles reading whether the function ended with errors.
func (funcCtx *batchFuncContext) extractResultsAndErr(numResps int, out []reflect.Value, retType graphql.Type) ([]interface{}, error) {
	if funcCtx.hasError {
		if err := out[len(out)-1]; !err.IsNil() {
			return nil, err.Interface().(error)
		}
	}
	if !funcCtx.hasRet {
		res := make([]interface{}, numResps)
		for i := 0; i < numResps; i++ {
			res[i] = true
		}
		return res, nil
	}
	resBatch := out[0]

	res := make([]interface{}, numResps)
	for _, mapKey := range resBatch.MapKeys() {
		res[mapKey.Int()] = resBatch.MapIndex(mapKey).Interface()
	}
	return res, nil
}
