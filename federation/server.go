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

type GrpcExecutorClient struct {
	Client thunderpb.ExecutorClient
}

func (c *GrpcExecutorClient) Execute(ctx context.Context, req *graphql.Query) ([]byte, error) {
	marshaled, err := marshalQuery(req)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.Execute(ctx, &thunderpb.ExecuteRequest{
		Query: marshaled,
	})
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

type DirectExecutorClient struct {
	Client thunderpb.ExecutorServer
}

func (c *DirectExecutorClient) Execute(ctx context.Context, req *graphql.Query) ([]byte, error) {
	// fmt.Println("MAOOPO")
	marshaled, err := marshalQuery(req)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.Execute(ctx, &thunderpb.ExecuteRequest{
		Query: marshaled,
	})
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

// Server must implement thunderpb.ExecutorServer.
var _ thunderpb.ExecutorServer = &Server{}

type Server struct {
	schema *graphql.Schema
}

func NewServer(schema *graphql.Schema) (*Server, error) {
	introspection.AddIntrospectionToSchema(schema)

	return &Server{
		schema: schema,
	}, nil
}

// Execute unmarshals the protobuf query and executes it on the server
func (s *Server) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	query, err := unmarshalQuery(req.Query)
	if err != nil {
		return nil, err
	}

	var schema graphql.Type
	switch query.Kind {
	case "query":
		schema = s.schema.Query
	case "mutation":
		schema = s.schema.Mutation
	default:
		return nil, fmt.Errorf("unknown kind %s", query.Kind)
	}

	// fmt.Println("BEFORE")
	// printSelections(query.Selections[0].SelectionSet)

	// fmt.Println("--------------")
	// printSelections(query.SelectionSet)
	// fmt.Println("--------------")
	if err := graphql.PrepareQuery(context.Background(), schema, query.SelectionSet); err != nil {
		return nil, err
	}

	// fmt.Println("AFTER")
	// printSelections(query.Selections[0].SelectionSet)

	// printSelections(query.SelectionSet)
	// fmt.Println("--------------")

	// Run subquery with the reactive cache
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
		res, err := gqlExec.Execute(ctx, schema, nil, query)
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

// marshalPbSelections gets a selection set and marshals it into the protobuf format
func marshalPbSelections(selectionSet *graphql.SelectionSet) (*thunderpb.SelectionSet, error) {
	if selectionSet == nil {
		return nil, nil
	}

	selections := make([]*thunderpb.Selection, 0, len(selectionSet.Selections))
	for _, selection := range selectionSet.Selections {
		children, err := marshalPbSelections(selection.SelectionSet)
		if err != nil {
			return nil, err
		}

		var args []byte
		if selection.Args != nil {
			var err error
			args, err = json.Marshal(selection.Args)
			if err != nil {
				return nil, err
			}
		}

		selections = append(selections, &thunderpb.Selection{
			Name:         selection.Name,
			Alias:        selection.Alias,
			SelectionSet: children,
			Arguments:    args,
		})
	}

	fragments := make([]*thunderpb.Fragment, 0, len(selectionSet.Fragments))
	for _, fragment := range selectionSet.Fragments {
		selections, err := marshalPbSelections(fragment.SelectionSet)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, &thunderpb.Fragment{
			On:           fragment.On,
			SelectionSet: selections,
		})
	}

	return &thunderpb.SelectionSet{
		Selections: selections,
		Fragments:  fragments,
	}, nil
}

// unmarshalPbSelectionSet gets a protobuf for a selection set and unmarshal it into graphql selection set object
func unmarshalPbSelectionSet(selectionSet *thunderpb.SelectionSet) (*graphql.SelectionSet, error) {
	if selectionSet == nil {
		return nil, nil
	}

	selections := make([]*graphql.Selection, 0, len(selectionSet.Selections))
	for _, selection := range selectionSet.Selections {
		children, err := unmarshalPbSelectionSet(selection.SelectionSet)
		if err != nil {
			return nil, err
		}

		var args map[string]interface{}
		if len(selection.Arguments) != 0 {
			if err := json.Unmarshal(selection.Arguments, &args); err != nil {
				return nil, err
			}
		}

		selections = append(selections, &graphql.Selection{
			Name:         selection.Name,
			Alias:        selection.Alias,
			SelectionSet: children,
			Args:         args,
		})
	}

	fragments := make([]*graphql.Fragment, 0, len(selectionSet.Fragments))
	for _, fragment := range selectionSet.Fragments {
		selections, err := unmarshalPbSelectionSet(fragment.SelectionSet)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, &graphql.Fragment{
			On:           fragment.On,
			SelectionSet: selections,
		})
	}

	return &graphql.SelectionSet{
		Selections: selections,
		Fragments:  fragments,
	}, nil
}

// marshalQuery marshals a graphql query type into a protobuf
func marshalQuery(query *graphql.Query) (*thunderpb.Query, error) {
	selectionSet, err := marshalPbSelections(query.SelectionSet)
	if err != nil {
		return nil, err
	}
	return &thunderpb.Query{
		Name:         query.Name,
		Kind:         query.Kind,
		SelectionSet: selectionSet,
	}, nil
}

// unmarshalQuery unmarshals a protobuf query type into the graphql query type
func unmarshalQuery(query *thunderpb.Query) (*graphql.Query, error) {
	selectionSet, err := unmarshalPbSelectionSet(query.SelectionSet)
	if err != nil {
		return nil, err
	}
	return &graphql.Query{
		Name:         query.Name,
		Kind:         query.Kind,
		SelectionSet: selectionSet,
	}, nil
}
