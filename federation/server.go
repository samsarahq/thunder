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

func unmarshalPbSelectionSet(selectionSet *thunderpb.SelectionSet) (*SelectionSet, error) {
	if selectionSet == nil {
		return nil, nil
	}

	selections := make([]*Selection, 0, len(selectionSet.Selections))
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

		selections = append(selections, &Selection{
			Name:         selection.Name,
			Alias:        selection.Alias,
			SelectionSet: children,
			Args:         args,
		})
	}

	fragments := make([]*Fragment, 0, len(selectionSet.Fragments))
	for _, fragment := range selectionSet.Fragments {
		selections, err := unmarshalPbSelectionSet(fragment.SelectionSet)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, &Fragment{
			On:           fragment.On,
			SelectionSet: selections,
		})
	}

	return &SelectionSet{
		Selections: selections,
		Fragments:  fragments,
	}, nil
}

func marshalPbSelections(selectionSet *SelectionSet) (*thunderpb.SelectionSet, error) {
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

func (s *Server) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	selectionSet, err := unmarshalPbSelectionSet(req.SelectionSet)
	if err != nil {
		return nil, err
	}

	gqlSelectionSet := convertSelectionSet(selectionSet)

	gqlExec := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	res, err := gqlExec.Execute(context.Background(), s.schema.Query, nil, &graphql.Query{
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
