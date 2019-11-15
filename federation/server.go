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
	schema      *graphql.Schema
	schemaBytes []byte
}

func NewServer(schema *graphql.Schema) (*Server, error) {
	introspectionSchema := introspection.BareIntrospectionSchema(schema)
	bytes, err := introspection.RunIntrospectionQuery(introspectionSchema)
	if err != nil {
		return nil, fmt.Errorf("get introspection result: %v", err)
	}

	return &Server{
		schema:      schema,
		schemaBytes: bytes,
	}, nil
}

func (s *Server) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	selectionSet, err := unmarshalPbSelectionSet(req.SelectionSet)
	if err != nil {
		return nil, err
	}

	gqlSelectionSet := convertSelectionSet(selectionSet)

	gqlExec := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	res, err := gqlExec.Execute(context.Background(), s.schema.Query, &graphql.Query{
		Kind:         "query",
		Name:         "",
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

func (s *Server) Schema(ctx context.Context, req *thunderpb.SchemaRequest) (*thunderpb.SchemaResponse, error) {
	return &thunderpb.SchemaResponse{
		Schema: s.schemaBytes,
	}, nil
}
