package federation

import (
	"context"
	"encoding/json"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/thunder/graphql/introspection"
)

// SchemaSyncer has a function that checks if the schema has changed,
// and if so updates the planner in the federated executor
type SchemaSyncer interface {
	FetchPlannerAndIntrospectionQueryResult(ctx context.Context) (*Planner, []byte, error)
}
type IntrospectionSchemaSyncer struct {
	executors     map[string]ExecutorClient
	queryMetadata interface{}
}

// Creates a schema syncer that periodically runs an introspection query agaisnt all the federated servers to check for updates.
func NewIntrospectionSchemaSyncer(ctx context.Context, executors map[string]ExecutorClient, queryMetadata interface{}) *IntrospectionSchemaSyncer {
	ss := &IntrospectionSchemaSyncer{
		executors:     executors,
		queryMetadata: queryMetadata,
	}
	return ss
}

func (s *IntrospectionSchemaSyncer) FetchPlannerAndIntrospectionQueryResult(ctx context.Context) (*Planner, []byte, error) {
	schemas := make(map[string]*IntrospectionQueryResult)
	for server, client := range s.executors {
		resp, err := fetchSchema(ctx, client, s.queryMetadata)
		if err != nil {
			return nil, nil, oops.Wrapf(err, "fetching schema %s", server)
		}
		schema := resp.Result
		var iq IntrospectionQueryResult
		if err := json.Unmarshal(schema, &iq); err != nil {
			return nil, nil, oops.Wrapf(err, "unmarshaling schema %s", server)
		}

		schemas[server] = &iq
	}

	types, err := convertSchema(schemas)
	if err != nil {
		return nil, nil, oops.Wrapf(err, "converting schemas error")
	}

	introspectionSchema := introspection.BareIntrospectionSchema(types.Schema)
	schema, err := introspection.RunIntrospectionQuery(introspection.BareIntrospectionSchema(introspectionSchema))
	if err != nil || schema == nil {
		return nil, nil, oops.Wrapf(err, "error running introspection query")
	}

	var iq IntrospectionQueryResult
	if err := json.Unmarshal(schema, &iq); err != nil {
		return nil, nil, oops.Wrapf(err, "unmarshaling introspection schema")
	}

	schemas[IntrospectionClientName] = &iq

	types, err = convertSchema(schemas)
	if err != nil {
		return nil, nil, oops.Wrapf(err, "converting schemas error")
	}

	planner, err := NewPlanner(types, nil)
	if err != nil {
		return nil, nil, oops.Wrapf(err, "creating planner")
	}

	return planner, schema, nil
}
