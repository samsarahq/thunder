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


