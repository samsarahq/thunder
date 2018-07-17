package schemabuilder

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"encoding/base64"
	"github.com/samsarahq/thunder/graphql"
)

// Connection conforms to the GraphQL Connection type in the Relay Pagination spec.
type Connection struct {
	Count    int64
	Edges    []Edge
	PageInfo PageInfo
}

// PageInfo contains information for pagination on a connection type. The list of Pages is used for
// page-number based pagination where the ith index corresponds to the start cursor of (i+1)st page.
type PageInfo struct {
	HasNextPage bool
	EndCursor   string
	HasPrevPage bool
	StartCursor string
	Pages       []string
}

// Edge consists of a node paired with its b64 encoded cursor.
type Edge struct {
	Node   interface{}
	Cursor string
}

// ConnectionArgs conform to the pagination arguments as specified by the Relay Spec for Connection
// types. The Args field consits of the user-facing args.
type ConnectionArgs struct {
	First  *int64
	Last   *int64
	After  *string
	Before *string
	Args   interface{}
}

// constructEdgeType wraps the typ (which is the type of the Node) in an Edge type conforming to the
// Relay spec.
func (sb *schemaBuilder) constructEdgeType(typ reflect.Type) (graphql.Type, error) {

	nodeType, err := sb.getType(typ)
	if err != nil {
		return nil, err
	}

	fieldMap := make(map[string]*graphql.Field)

	nodeField := &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			if value, ok := source.(Edge); ok {
				return value.Node, nil
			}

			return nil, fmt.Errorf("error resolving node in edge")

		},
		Type:           nodeType,
		ParseArguments: nilParseArguments,
	}
	fieldMap["node"] = nodeField

	cursorType, err := sb.getType(reflect.TypeOf(string("")))
	if err != nil {
		return nil, err
	}

	cursorField := &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			if value, ok := source.(Edge); ok {
				return value.Cursor, nil
			}
			return nil, fmt.Errorf("error resolving cursor in edge")
		},
		Type:           cursorType,
		ParseArguments: nilParseArguments,
	}

	fieldMap["cursor"] = cursorField

	return &graphql.NonNull{
		Type: &graphql.Object{
			Name:        "Edge",
			Description: "",
			Fields:      fieldMap,
		},
	}, nil

}

// constructConnType wraps typ (type of the Node) in a Connection Type conforming to the Relay spec.
func (sb *schemaBuilder) constructConnType(typ reflect.Type) (graphql.Type, error) {

	fieldMap := make(map[string]*graphql.Field)

	countType, _ := reflect.TypeOf(Connection{}).FieldByName("Count")
	countField, err := sb.buildField(countType)
	if err != nil {
		return nil, err
	}

	fieldMap["count"] = countField

	edgeType, err := sb.constructEdgeType(typ)
	if err != nil {
		return nil, err
	}

	edgesSliceType := &graphql.NonNull{Type: &graphql.List{Type: edgeType}}

	edgesSliceField := &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			if value, ok := source.(Connection); ok {
				return value.Edges, nil
			}
			return nil, fmt.Errorf("error resolving edges in connection")
		},
		Type:           edgesSliceType,
		ParseArguments: nilParseArguments,
	}

	fieldMap["edges"] = edgesSliceField

	pageInfoType, _ := reflect.TypeOf(Connection{}).FieldByName("PageInfo")
	pageInfoField, err := sb.buildField(pageInfoType)
	if err != nil {
		return nil, err
	}
	fieldMap["pageInfo"] = pageInfoField

	retObject := &graphql.NonNull{Type: &graphql.Object{Name: "Connection", Description: "", Fields: fieldMap}}
	return retObject, nil
}

// EdgesToReturn returns the slice of edges by appyling the pagination arguments. It also returns
// the hasNextPage and hasPrevPage values respectively. The behavior is expected to conform to the
// Relay Cursor spec: https://facebook.github.io/relay/graphql/connections.htm#EdgesToReturn()
func EdgesToReturn(allEdges []Edge, before *string, after *string, first *int64, last *int64) ([]Edge, bool, bool, error) {
	edges, elemsAfter, elemsBefore := applyCursorsToAllEdges(allEdges, before, after)

	prevPage := false
	nextPage := false

	if first != nil {
		if *first < 0 {
			return nil, nextPage, prevPage, graphql.NewClientError("first should be a non-negative integer")
		}
		if len(edges) > int(*first) {
			edges = edges[:int(*first)]
			nextPage = true
		}
	}
	if before != nil {
		if elemsAfter {
			nextPage = true
		}
	}

	if last != nil {
		if *last < 0 {
			return nil, nextPage, prevPage, graphql.NewClientError("last should be a non-negative integer")
		}
		if len(edges) > int(*last) {
			edges = edges[len(edges)-int(*last):]
			prevPage = true
		}
	}
	if after != nil {
		if elemsBefore {
			prevPage = true
		}
	}

	return edges, nextPage, prevPage, nil
}

// getCursorIndex returns the index corresponding to the cursor in the slice.
func getCursorIndex(edges []Edge, cursor string) int {
	for i, val := range edges {
		if val.Cursor == cursor {
			return i
		}
	}
	return -1
}

// applyCursorsToAllEdges returns the slice of edges after applying the after and before arguments.
// It also implements part of the hasNextPage and hasPrevPage algorithm by returning if there are
// elements after or before the arguments.
func applyCursorsToAllEdges(allEdges []Edge, before *string, after *string) ([]Edge, bool, bool) {
	edges := allEdges

	elemsAfter := false
	elemsBefore := false

	if after != nil {
		i := getCursorIndex(edges, *after)
		if i != -1 {
			edges = edges[i+1:]
			if i != 0 {
				elemsBefore = true
			}
		}

	}
	if before != nil {
		i := getCursorIndex(edges, *before)
		if i != -1 {
			edges = edges[:i]
			if i != len(allEdges)-1 {
				elemsAfter = true
			}
		}

	}

	return edges, elemsAfter, elemsBefore

}

// getConnection applies the ConnectionArgs to nodes and returns the result in a wrapped Connection
// type.
func getConnection(key string, nodes []interface{}, args ConnectionArgs) (Connection, error) {
	var edges []Edge

	lim := int64(0)
	if args.First != nil {
		lim = *args.First
	} else if args.Last != nil {
		lim = *args.Last
	}

	var pages []string
	for i, val := range nodes {
		// Get the value of the key field and then b64 encode it for the cursor.
		keyValue := reflect.ValueOf(val)
		if keyValue.Kind() == reflect.Ptr {
			keyValue = keyValue.Elem()
		}
		keyString := []byte(fmt.Sprintf("%v", keyValue.FieldByName(key).Interface()))
		cursorVal := base64.StdEncoding.EncodeToString(keyString)
		if (int64(i) % lim) == 0 {
			pages = append(pages, cursorVal)
		}
		edges = append(edges, Edge{Node: val, Cursor: cursorVal})
	}

	edges, nextPage, prevPage, err := EdgesToReturn(edges, args.Before, args.After, args.First, args.Last)
	if err != nil {
		return Connection{}, err
	}

	endCursor := ""
	if len(edges) > 0 {
		endCursor = edges[len(edges)-1].Cursor
	}
	startCursor := ""
	if len(edges) > 0 {
		startCursor = edges[0].Cursor
	}

	pageInfo := PageInfo{HasNextPage: nextPage, EndCursor: endCursor, StartCursor: startCursor, HasPrevPage: prevPage, Pages: pages}

	return Connection{Count: int64(len(nodes)), Edges: edges, PageInfo: pageInfo}, nil
}

// PaginateFieldFunc registers a function that is also paginated according to the Relay
// Connection Spec. The field is registered as a Connection Type and first, last, before and after
// are automatically added as arguments to the function. The return type to the function must be a
// list. The element of the list is wrapped as a Node Type.
func (o *Object) PaginateFieldFunc(name string, f interface{}) {
	o.PaginatedFields = append(o.PaginatedFields,
		PaginationObject{
			Name: name,
			Fn:   f,
		})
}

// buildPaginatedField corresponds to buildFunction on a paginated type. It wraps the return result
// of f in a connection type.
func (sb *schemaBuilder) buildPaginatedField(typ reflect.Type, f interface{}) (*graphql.Field, error) {
	fun := reflect.ValueOf(f)

	ptr := reflect.PtrTo(typ)

	if fun.Kind() != reflect.Func {
		return nil, fmt.Errorf("fun must be func, not %s", fun)
	}

	funcType := fun.Type()

	in := make([]reflect.Type, 0, funcType.NumIn())
	for i := 0; i < funcType.NumIn(); i++ {
		in = append(in, funcType.In(i))
	}

	var argParser *argParser
	var argType graphql.Type
	var ptrFunc bool
	var hasContext, hasSource, hasSelectionSet bool

	if len(in) > 0 && in[0] == contextType {
		hasContext = true
		in = in[1:]
	}
	if len(in) > 0 && (in[0] == typ || in[0] == ptr) {
		hasSource = true
		ptrFunc = in[0] == ptr
		in = in[1:]
	}

	if len(in) > 0 && in[0] != selectionSetType {
		var err error
		if argParser, argType, err = sb.buildPaginatedArgParser(in[0]); err != nil {
			return nil, fmt.Errorf("attempted to wrap %s as arguments struct, but failed: %s", in[0].Name(), err.Error())
		}
		in = in[1:]
	} else {
		var err error
		if argParser, argType, err = sb.buildPaginatedArgParser(nil); err != nil {
			return nil, fmt.Errorf("test this case...shouldn't be failing in any case")
		}
	}

	if len(in) > 0 && in[0] == selectionSetType {
		hasSelectionSet = true
		in = in[:len(in)-1]
	}

	// We have succeeded if no arguments remain.
	if len(in) != 0 {
		return nil, fmt.Errorf("%s arguments should be [context][, [*]%s][, args][, selectionSet]", funcType, typ)
	}

	// Parse return values. The first return value must be the actual value, and
	// the second value can optionally be an error.

	out := make([]reflect.Type, 0, funcType.NumOut())
	for i := 0; i < funcType.NumOut(); i++ {
		out = append(out, funcType.Out(i))
	}

	var hasRet, hasError bool

	var nodeType reflect.Type
	if len(out) > 0 && out[0] != errType {
		nodeType = out[0].Elem()
		hasRet = true
		out = out[1:]
	} else {
		return nil, fmt.Errorf("paginated function must have a return argument")
	}

	if len(out) > 0 && out[0] == errType {
		hasError = true
		out = out[1:]
	}

	if len(out) != 0 {
		return nil, fmt.Errorf("%s return values should [result][, error]", funcType)
	}

	var retType graphql.Type
	if hasRet {
		var err error
		retType, err = sb.constructConnType(nodeType)
		if err != nil {
			return nil, err
		}

	} else {
		var err error
		retType, err = sb.getType(reflect.TypeOf(true))
		if err != nil {
			return nil, err
		}
	}

	// If the nodeType isn't registered it might be of a pointer type
	nodeObj := sb.objects[nodeType]
	if nodeObj == nil && nodeType.Kind() == reflect.Ptr {
		nodeObj = sb.objects[nodeType.Elem()]
	}
	nodeKey := nodeObj.key
	if nodeKey == "" {
		return nil, fmt.Errorf("a key field should be registered on the return object")
	}

	args := make(map[string]graphql.Type)

	inputObject, ok := argType.(*graphql.InputObject)
	if !ok {
		return nil, fmt.Errorf("%s's args should be an object", funcType)
	}

	for name, nodeType := range inputObject.InputFields {
		args[name] = nodeType
	}

	ret := &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {

			if _, ok := args.(ConnectionArgs); !ok {
				return nil, fmt.Errorf("arguments should implement ConnectionArgs")
			}

			in := make([]reflect.Value, 0, funcType.NumIn())

			if hasContext {
				in = append(in, reflect.ValueOf(ctx))
			}

			// Set up source.
			if hasSource {
				sourceValue := reflect.ValueOf(source)
				ptrSource := sourceValue.Kind() == reflect.Ptr
				switch {
				case ptrSource && !ptrFunc:
					in = append(in, sourceValue.Elem())
				case !ptrSource && ptrFunc:
					copyPtr := reflect.New(typ)
					copyPtr.Elem().Set(sourceValue)
					in = append(in, copyPtr)
				default:
					in = append(in, sourceValue)
				}
			}

			// Set up other arguments.
			if val, _ := args.(ConnectionArgs); val.Args != nil {
				in = append(in, reflect.ValueOf(val.Args).Elem())
			}

			if hasSelectionSet {
				in = append(in, reflect.ValueOf(selectionSet))
			}

			// Call the function.
			out := fun.Call(in)
			var result interface{}
			if hasRet {
				var err error
				connectionArgs, _ := args.(ConnectionArgs)

				result, err = getConnection(nodeKey, castSlice(out[0].Interface()), connectionArgs)
				if err != nil {
					return nil, err
				}

				out = out[1:]
			} else {
				result = true
			}
			if hasError {
				if err := out[0]; !err.IsNil() {
					return nil, err.Interface().(error)
				}
			}

			if _, ok := retType.(*graphql.NonNull); ok {
				resultValue := reflect.ValueOf(result)
				if resultValue.Kind() == reflect.Ptr && resultValue.IsNil() {
					return nil, fmt.Errorf("%s is marked non-nullable but returned a null value", funcType)
				}
			}

			return result, nil
		},
		Args:           args,
		Type:           retType,
		ParseArguments: argParser.Parse,
		Expensive:      false,
	}

	return ret, nil
}

func castSlice(slice interface{}) []interface{} {
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice {
		panic("cast given a non-slice type")
	}

	ret := make([]interface{}, s.Len())
	for i := 0; i < s.Len(); i++ {
		ret[i] = s.Index(i).Interface()
	}

	return ret
}

// buildPaginatedArgParser corresponds to buildArgParser for arguments used on a paginated
// fieldFunc. The args are nested as the Args field in the ConnectionArgs.
func (sb *schemaBuilder) buildPaginatedArgParser(originalArgType reflect.Type) (*argParser, graphql.Type, error) {
	//nestedArgParser and nestedArgType are used for building the parser function for the args
	//passed in to the paginated field.
	typ := reflect.TypeOf(ConnectionArgs{})

	// Fields build a map of the fields for ConnectionArgs.
	fields := make(map[string]argField)

	argType := &graphql.InputObject{
		Name:        typ.Name(),
		InputFields: make(map[string]graphql.Type),
	}

	argType.Name += "_InputObject"

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// The field which is of type interface should only be one and will be used to parse the
		// original function args.
		if field.Type.Kind() == reflect.Interface {
			continue
		}

		name := makeGraphql(field.Name)

		var parser *argParser
		var fieldArgTyp graphql.Type

		parser, fieldArgTyp, err := sb.makeArgParser(field.Type)
		if err != nil {
			return nil, nil, err
		}

		argType.InputFields[name] = fieldArgTyp

		fields[name] = argField{
			field:  field,
			parser: parser,
		}
	}

	var nestedArgParser *argParser
	var nestedArgType graphql.Type
	var err error
	if originalArgType != nil {
		nestedArgParser, nestedArgType, err = sb.makeStructParser(originalArgType)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build args for paginated field")
		}
		userInputObject, ok := nestedArgType.(*graphql.InputObject)
		if !ok {
			return nil, nil, fmt.Errorf("args should be an object")
		}

		for name, typ := range userInputObject.InputFields {
			argType.InputFields[name] = typ
		}
	}

	return &argParser{
		FromJSON: func(value interface{}, dest reflect.Value) error {
			asMap, ok := value.(map[string]interface{})
			if !ok {
				return errors.New("not an object")
			}

			for name, field := range fields {
				value := asMap[name]
				fieldDest := dest.FieldByIndex(field.field.Index)
				if err := field.parser.FromJSON(value, fieldDest); err != nil {
					return fmt.Errorf("%s: %s", name, err)
				}
			}

			// nestedArgFields is the map used to parse the remaining fields: any field which isn't
			// part of ConnectionArgs should be a field of the args used for the paginated field.
			nestedArgFields := make(map[string]interface{})
			for name := range asMap {
				if _, ok := fields[name]; !ok {
					nestedArgFields[name] = asMap[name]
				}
			}

			if nestedArgParser == nil {
				if len(nestedArgFields) != 0 {
					return fmt.Errorf("error in parsing args")
				}
				return nil
			}

			fieldDest := dest.FieldByName("Args")
			tmpDest := reflect.New(nestedArgParser.Type)
			if err := nestedArgParser.FromJSON(nestedArgFields, tmpDest.Elem()); err != nil {
				return err
			}
			fieldDest.Set(tmpDest)

			return nil
		},
		Type: typ,
	}, argType, nil
}
