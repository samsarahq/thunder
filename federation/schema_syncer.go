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
	FetchPlanner(ctx context.Context, optionalArgs interface{}) (*Planner, error)
}
type IntrospectionSchemaSyncer struct {
	executors    map[string]ExecutorClient
	optionalArgs interface{}
}

// Creates a schema syncer that periodically runs an introspection query agaisnt all the federated servers to check for updates.
func NewIntrospectionSchemaSyncer(ctx context.Context, executors map[string]ExecutorClient, optionalArgs interface{}) *IntrospectionSchemaSyncer {
	ss := &IntrospectionSchemaSyncer{
		executors:    executors,
		optionalArgs: optionalArgs,
	}
	return ss
}

func (s *IntrospectionSchemaSyncer) FetchPlanner(ctx context.Context, optionalArgs interface{}) (*Planner, error) {
	schemas := make(map[string]*introspectionQueryResult)
	for server, client := range s.executors {
		resp, err := fetchSchema(ctx, client, optionalArgs)
		schema := resp.Result
		if err != nil {
			return nil, oops.Wrapf(err, "fetching schema %s", server)
		}
		var iq introspectionQueryResult
		if err := json.Unmarshal(schema, &iq); err != nil {
			return nil, oops.Wrapf(err, "unmarshaling schema %s", server)
		}

		schemas[server] = &iq
	}

	types, err := convertSchema(schemas)
	if err != nil {
		return nil, oops.Wrapf(err, "converting schemas error")
	}

	introspectionSchema := introspection.BareIntrospectionSchema(types.Schema)
	schema, err := introspection.RunIntrospectionQuery(introspection.BareIntrospectionSchema(introspectionSchema))
	if err != nil || schema == nil {
		return nil, oops.Wrapf(err, "error running introspection query")
	}

	var iq introspectionQueryResult
	if err := json.Unmarshal(schema, &iq); err != nil {
		return nil, oops.Wrapf(err, "unmarshaling introspection schema")
	}

	schemas["introspection"] = &iq
	types, err = convertSchema(schemas)
	if err != nil {
		return nil, oops.Wrapf(err, "converting schemas error")
	}

	return NewPlanner(types)
}
