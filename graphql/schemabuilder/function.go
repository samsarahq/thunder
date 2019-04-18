package schemabuilder

import (
	"context"
	"fmt"
	"reflect"

	"github.com/samsarahq/thunder/graphql"
)

// buildFunction takes the reflect type of an object and a method attached to
// that object to build a GraphQL Field that can be resolved in the GraphQL
// graph.
func (sb *schemaBuilder) buildFunction(typ reflect.Type, m *method) (*graphql.Field, error) {
	funcCtx := &funcContext{typ: typ}

	if typ.Kind() == reflect.Ptr {
		return nil, fmt.Errorf("source-type of buildFunction cannot be a pointer (got: %v)", typ)
	}

	callableFunc, err := funcCtx.getFuncVal(m)
	if err != nil {
		return nil, err
	}

	in := funcCtx.getFuncInputTypes()
	in = funcCtx.consumeContextAndSource(in)

	argParser, argType, in, err := funcCtx.getArgParserAndTyp(sb, in)
	if err != nil {
		return nil, err
	}
	funcCtx.hasArgs = argParser != nil

	in = funcCtx.consumeSelectionSet(in)

	// We have succeeded if no arguments remain.
	if len(in) != 0 {
		return nil, fmt.Errorf("%s arguments should be [context][, [*]%s][, args][, selectionSet]", funcCtx.funcType, typ)
	}

	// Parse return values. The first return value must be the actual value, and
	// the second value can optionally be an error.
	err = funcCtx.parseReturnSignature(m)
	if err != nil {
		return nil, err
	}

	retType, err := funcCtx.getReturnType(sb, m)
	if err != nil {
		return nil, err
	}

	args, err := funcCtx.argsTypeMap(argType)
	if err != nil {
		return nil, err
	}

	return &graphql.Field{
		Resolve: func(ctx context.Context, source, funcRawArgs interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			// Set up function arguments.
			funcInputArgs := funcCtx.prepareResolveArgs(source, funcRawArgs, ctx, selectionSet)

			// Call the function.
			funcOutputArgs := callableFunc.Call(funcInputArgs)

			return funcCtx.extractResultAndErr(funcOutputArgs, retType)

		},
		Args:           args,
		Type:           retType,
		ParseArguments: argParser.Parse,
		Expensive:      funcCtx.hasContext,
	}, nil
}

// funcContext is used to parse the function signature in buildFunction.
type funcContext struct {
	hasContext      bool
	hasSource       bool
	hasArgs         bool
	hasSelectionSet bool
	hasRet          bool
	hasError        bool

	funcType     reflect.Type
	isPtrFunc    bool
	typ          reflect.Type
	selectionSet *graphql.SelectionSet
}

// getFuncVal returns a reflect.Value of an executable function.
func (funcCtx *funcContext) getFuncVal(m *method) (reflect.Value, error) {
	fun := reflect.ValueOf(m.Fn)
	if fun.Kind() != reflect.Func {
		return fun, fmt.Errorf("fun must be func, not %s", fun)
	}
	funcCtx.funcType = fun.Type()
	return fun, nil
}

// getFuncInputTypes returns the input arguments for the function we're
// representing.
func (funcCtx *funcContext) getFuncInputTypes() []reflect.Type {
	in := make([]reflect.Type, 0, funcCtx.funcType.NumIn())
	for i := 0; i < funcCtx.funcType.NumIn(); i++ {
		in = append(in, funcCtx.funcType.In(i))
	}
	return in
}

// consumeContextAndSource reads in the input parameters for the provided
// function and determines whether the function has a Context input parameter
// and/or whether it includes the "source" input parameter ("source" will be the
// object type that this function is connected to).  If we find either of these
// fields we will pop that field from the input parameters we return (since we've
// already "dealt" with those fields).
func (funcCtx *funcContext) consumeContextAndSource(in []reflect.Type) []reflect.Type {
	ptr := reflect.PtrTo(funcCtx.typ)

	if len(in) > 0 && in[0] == contextType {
		funcCtx.hasContext = true
		in = in[1:]
	}

	if len(in) > 0 && (in[0] == funcCtx.typ || in[0] == ptr) {
		funcCtx.hasSource = true
		funcCtx.isPtrFunc = in[0] == ptr
		in = in[1:]
	}

	return in
}

// getArgParserAndTyp reads a list of input parameters, and, if we have a set
// of custom parameters for the field func (at this point any input type other
// than the selectionSet is considered the args input), we will return the
// argParser for that type and pop that field from the returned input parameters.
func (funcCtx *funcContext) getArgParserAndTyp(sb *schemaBuilder, in []reflect.Type) (*argParser, graphql.Type, []reflect.Type, error) {
	var argParser *argParser
	var argType graphql.Type
	if len(in) > 0 && in[0] != selectionSetType {
		var err error
		if argParser, argType, err = sb.makeStructParser(in[0]); err != nil {
			return nil, nil, in, fmt.Errorf("attempted to parse %s as arguments struct, but failed: %s", in[0].Name(), err.Error())
		}
		in = in[1:]
	}
	return argParser, argType, in, nil
}

// consumeSelectionSet reads the input parameters and will pop off the
// selectionSet type if we detect it in the input fields.  Check out
// graphql.SelectionSet for more infomation about selection sets.
func (funcCtx *funcContext) consumeSelectionSet(in []reflect.Type) []reflect.Type {
	if len(in) > 0 && in[0] == selectionSetType {
		in = in[:len(in)-1]
		funcCtx.hasSelectionSet = true
		return in
	}
	funcCtx.hasSelectionSet = false
	return in
}

// parseReturnSignature reads and validates the return signature of the function
// to determine whether it has a return type and/or an error response.
func (funcCtx *funcContext) parseReturnSignature(m *method) (err error) {
	out := make([]reflect.Type, 0, funcCtx.funcType.NumOut())
	for i := 0; i < funcCtx.funcType.NumOut(); i++ {
		out = append(out, funcCtx.funcType.Out(i))
	}

	if len(out) > 0 && out[0] != errType {
		funcCtx.hasRet = true
		out = out[1:]
	}

	if len(out) > 0 && out[0] == errType {
		funcCtx.hasError = true
		out = out[1:]
	}

	if len(out) != 0 {
		err = fmt.Errorf("%s return values should [result][, error]", funcCtx.funcType)
		return
	}

	if !funcCtx.hasRet && m.MarkedNonNullable {
		err = fmt.Errorf("%s is marked non-nullable, but has no return value", funcCtx.funcType)
		return
	}
	return
}

// getReturnType returns a GraphQL node type for the return type of the
// function.  So an object "User" that has a linked function which returns a
// list of "Hats" will resolve the GraphQL type of a "Hat" at this point.
func (funcCtx *funcContext) getReturnType(sb *schemaBuilder, m *method) (graphql.Type, error) {
	var retType graphql.Type
	if funcCtx.hasRet {
		var err error
		retType, err = sb.getType(funcCtx.funcType.Out(0))
		if err != nil {
			return nil, err
		}

		if m.MarkedNonNullable {
			if _, ok := retType.(*graphql.NonNull); !ok {
				retType = &graphql.NonNull{Type: retType}
			}
		}
	} else {
		var err error
		retType, err = sb.getType(reflect.TypeOf(true))
		if err != nil {
			return nil, err
		}
	}
	return retType, nil
}

// argsTypeMap returns a map from input arg field names to a graphQL type
// associated with that field name.
func (funcCtx *funcContext) argsTypeMap(argType graphql.Type) (map[string]graphql.Type, error) {
	args := make(map[string]graphql.Type)
	if funcCtx.hasArgs {
		inputObject, ok := argType.(*graphql.InputObject)
		if !ok {
			return nil, fmt.Errorf("%s's args should be an object", funcCtx.funcType)
		}

		for name, typ := range inputObject.InputFields {
			args[name] = typ
		}
	}
	return args, nil
}

// prepareResolveArgs converts the provided source, args and context into the
// required list of reflect.Value types that the function needs to be called.
func (funcCtx *funcContext) prepareResolveArgs(source interface{}, args interface{}, ctx context.Context, selectionSet *graphql.SelectionSet) []reflect.Value {
	in := make([]reflect.Value, 0, funcCtx.funcType.NumIn())
	if funcCtx.hasContext {
		in = append(in, reflect.ValueOf(ctx))
	}

	// Set up source.
	if funcCtx.hasSource {
		sourceValue := reflect.ValueOf(source)
		ptrSource := sourceValue.Kind() == reflect.Ptr
		switch {
		case ptrSource && !funcCtx.isPtrFunc:
			in = append(in, sourceValue.Elem())
		case !ptrSource && funcCtx.isPtrFunc:
			copyPtr := reflect.New(funcCtx.typ)
			copyPtr.Elem().Set(sourceValue)
			in = append(in, copyPtr)
		default:
			in = append(in, sourceValue)
		}
	}

	// Set up other arguments.
	if funcCtx.hasArgs {
		in = append(in, reflect.ValueOf(args))
	}
	if funcCtx.hasSelectionSet {
		in = append(in, reflect.ValueOf(selectionSet))
	}

	return in
}

// extractResultAndErr converts the response from calling the function into
// the expected type for the response object (as opposed to a reflect.Value).
// It also handles reading whether the function ended with errors.
func (funcCtx *funcContext) extractResultAndErr(out []reflect.Value, retType graphql.Type) (interface{}, error) {
	var result interface{}
	if funcCtx.hasRet {
		result = out[0].Interface()
		out = out[1:]
	} else {
		result = true
	}
	if funcCtx.hasError {
		if err := out[0]; !err.IsNil() {
			return nil, err.Interface().(error)
		}
	}

	if _, ok := retType.(*graphql.NonNull); ok {
		resultValue := reflect.ValueOf(result)
		if resultValue.Kind() == reflect.Ptr && resultValue.IsNil() {
			return nil, fmt.Errorf("%s is marked non-nullable but returned a null value", funcCtx.funcType)
		}
	}

	return result, nil
}
