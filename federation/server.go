package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/thunderpb"
)

type Server struct {
	schema      *graphql.Schema
	schemaBytes []byte
}

func NewServer(schema *schemabuilder.Schema) (*Server, error) {
	built, err := schema.Build()
	if err != nil {
		return nil, fmt.Errorf("build schema: %v", err)
	}

	bytes, err := introspection.ComputeSchemaJSON(*schema)
	if err != nil {
		return nil, fmt.Errorf("get introspection result: %v", err)
	}

	return &Server{
		schema:      built,
		schemaBytes: bytes,
	}, nil
}

func unmarshalPbSelections(selections []*thunderpb.Selection) ([]*Selection, error) {
	if selections == nil {
		return nil, nil
	}

	result := make([]*Selection, 0, len(selections))
	for _, selection := range selections {
		children, err := unmarshalPbSelections(selection.Selections)
		if err != nil {
			return nil, err
		}

		var args map[string]interface{}
		if len(selection.Arguments) != 0 {
			if err := json.Unmarshal(selection.Arguments, &args); err != nil {
				return nil, err
			}
		}

		result = append(result, &Selection{
			Name:       selection.Name,
			Alias:      selection.Alias,
			Selections: children,
			Args:       args,
		})
	}

	return result, nil
}

func marshalPbSelections(selections []*Selection) ([]*thunderpb.Selection, error) {
	if selections == nil {
		return nil, nil
	}

	result := make([]*thunderpb.Selection, 0, len(selections))
	for _, selection := range selections {
		children, err := marshalPbSelections(selection.Selections)
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

		result = append(result, &thunderpb.Selection{
			Name:       selection.Name,
			Alias:      selection.Alias,
			Selections: children,
			Arguments:  args,
		})
	}

	return result, nil

}

func (s *Server) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	selections, err := unmarshalPbSelections(req.Selections)
	if err != nil {
		return nil, err
	}

	selectionSet := convertSelectionSet(selections)

	gqlExec := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	res, err := gqlExec.Execute(context.Background(), s.schema.Query, nil, &graphql.Query{
		Kind:         "query",
		Name:         "",
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
}

func (s *Server) Schema(ctx context.Context, req *thunderpb.SchemaRequest) (*thunderpb.SchemaResponse, error) {
	return &thunderpb.SchemaResponse{
		Schema: s.schemaBytes,
	}, nil
}
