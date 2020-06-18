package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/reactive"
	"github.com/samsarahq/thunder/thunderpb"
)

// DirectExecutorClient is used to execute directly on any of the graphql servers
type DirectExecutorClient struct {
	Client thunderpb.ExecutorServer
}

func (c *DirectExecutorClient) Execute(ctx context.Context, request *QueryRequest) (*QueryResponse, error) {
	// marshal query into a protobuf
	marshaled, err := MarshalQuery(request.Query)
	if err != nil {
		return nil, oops.Wrapf(err, "marshaling query")
	}
	// Make a request to the executor client with the query
	resp, err := c.Client.Execute(ctx, &thunderpb.ExecuteRequest{
		Query: marshaled,
	})
	if err != nil {
		return nil, oops.Wrapf(err, "executing query")
	}
	return &QueryResponse{Result: resp.Result}, nil
}

// Server must implement thunderpb.ExecutorServer.
var _ thunderpb.ExecutorServer = &Server{}

type Server struct {
	schema        *graphql.Schema
	localExecutor graphql.ExecutorRunner
}

func NewServer(schema *graphql.Schema) (*Server, error) {
	introspection.AddIntrospectionToSchema(schema)
	localExecutor := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	return &Server{
		schema:        schema,
		localExecutor: localExecutor,
	}, nil
}

// ExecuteRequest unmarshals the protobuf query and executes it on the server
func ExecuteRequest(ctx context.Context, req *thunderpb.ExecuteRequest, gqlSchema *graphql.Schema, localExecutor graphql.ExecutorRunner) (*thunderpb.ExecuteResponse, error) {
	query, err := UnmarshalQuery(req.Query)
	if err != nil {
		return nil, oops.Wrapf(err, "unmarshaling query")
	}

	var schema graphql.Type
	switch query.Kind {
	case "query":
		schema = gqlSchema.Query
	case "mutation":
		schema = gqlSchema.Mutation
	default:
		return nil, fmt.Errorf("unknown kind %s", query.Kind)
	}

	if err := graphql.PrepareQuery(ctx, schema, query.SelectionSet); err != nil {
		return nil, err
	}

	// We're using `reactive.NewRerunner` to ensure that the reactive cache is set up correctly,
	// but we won't actually wait for the query to rerun if invalidated.
	done := make(chan struct{})
	var queryResponse *thunderpb.ExecuteResponse
	var queryError error
	rerunner := reactive.NewRerunner(ctx, func(ctx context.Context) (ret interface{}, err error) {
		defer func() {
			queryResponse = ret.(*thunderpb.ExecuteResponse)
			queryError = err
			close(done)
		}()

		res, err := localExecutor.Execute(ctx, schema, nil, query)
		if err != nil {
			return nil, fmt.Errorf("executing query: %v", err)
		}

		bytes, err := json.Marshal(res)
		if err != nil {
			return nil, oops.Wrapf(err, "unmarshalling json query response")
		}

		return &thunderpb.ExecuteResponse{
			Result: bytes,
		}, nil
	}, time.Hour, false)

	<-done

	rerunner.Stop()
	return queryResponse, queryError
}

func (s *Server) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	return ExecuteRequest(ctx, req, s.schema, s.localExecutor)
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
			return nil, oops.Wrapf(err, "marshaling selections")
		}

		var args []byte
		if selection.UnparsedArgs != nil {
			var err error
			args, err = json.Marshal(selection.UnparsedArgs)
			if err != nil {
				return nil, oops.Wrapf(err, "marshaling args")
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
			return nil, oops.Wrapf(err, "marshaling fragments")
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
			return nil, oops.Wrapf(err, "unmarshaling selections")
		}

		var args map[string]interface{}
		if len(selection.Arguments) != 0 {
			if err := json.Unmarshal(selection.Arguments, &args); err != nil {
				return nil, oops.Wrapf(err, "unmarshaling selection arguments")
			}
		}

		selections = append(selections, &graphql.Selection{
			Name:         selection.Name,
			Alias:        selection.Alias,
			SelectionSet: children,
			UnparsedArgs: args,
		})
	}

	fragments := make([]*graphql.Fragment, 0, len(selectionSet.Fragments))
	for _, fragment := range selectionSet.Fragments {
		selections, err := unmarshalPbSelectionSet(fragment.SelectionSet)
		if err != nil {
			return nil, oops.Wrapf(err, "unmarshaling fragments")
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
func MarshalQuery(query *graphql.Query) (*thunderpb.Query, error) {
	selectionSet, err := marshalPbSelections(query.SelectionSet)
	if err != nil {
		return nil, oops.Wrapf(err, "marshaling query")
	}
	return &thunderpb.Query{
		Name:         query.Name,
		Kind:         query.Kind,
		SelectionSet: selectionSet,
	}, nil
}

// unmarshalQuery unmarshals a protobuf query type into the graphql query type
func UnmarshalQuery(query *thunderpb.Query) (*graphql.Query, error) {
	selectionSet, err := unmarshalPbSelectionSet(query.SelectionSet)
	if err != nil {
		return nil, oops.Wrapf(err, "unmarshaling query")
	}
	return &graphql.Query{
		Name:         query.Name,
		Kind:         query.Kind,
		SelectionSet: selectionSet,
	}, nil
}
