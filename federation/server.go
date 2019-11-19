package federation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/thunderpb"
)

type Server struct {
	schema *graphql.Schema
}

func NewServer(schema *graphql.Schema) (*Server, error) {
	introspection.AddIntrospectionToSchema(schema)

	return &Server{
		schema: schema,
	}, nil
}

func (s *Server) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	selectionSet, err := unmarshalPbSelectionSet(req.SelectionSet)
	if err != nil {
		return nil, err
	}

	gqlSelectionSet := convertSelectionSet(selectionSet)

	var kind string
	var schema graphql.Type
	switch req.Kind {
	case thunderpb.ExecuteRequest_QUERY:
		kind = "query"
		schema = s.schema.Query
	case thunderpb.ExecuteRequest_MUTATION:
		kind = "mutation"
		schema = s.schema.Mutation
	default:
		return nil, fmt.Errorf("unknown kind %s", req.Kind)
	}

	gqlExec := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	res, err := gqlExec.Execute(context.Background(), schema, &graphql.Query{
		Kind:         kind,
		Name:         req.Name,
		SelectionSet: gqlSelectionSet,
	})
	if err != nil {
		return nil, fmt.Errorf("executing query: %v", err)
	}

	bytes, err := json.Marshal(res)
	if err != nil {
		return nil, err
	}

	return &thunderpb.ExecuteResponse{
		Result: bytes,
	}, nil
}
