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

