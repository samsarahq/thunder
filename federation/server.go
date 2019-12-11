package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/reactive"
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

	// XXX: junk to have reactive.Cache work
	done := make(chan struct{}, 0)
	var r *thunderpb.ExecuteResponse
	var e error
	rerunner := reactive.NewRerunner(ctx, func(ctx context.Context) (ret interface{}, err error) {
		defer func() {
			r, _ = ret.(*thunderpb.ExecuteResponse)
			e = err
			close(done)
		}()

		gqlExec := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
		res, err := gqlExec.Execute(ctx, schema, &graphql.Query{
			Kind:         kind,
			Name:         req.Name,
			SelectionSet: selectionSet,
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
	}, time.Hour, false)
	<-done

	rerunner.Stop()
	return r, e
}
