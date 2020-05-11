package federation

import (
	"context"
	"encoding/json"

	"github.com/samsarahq/go/oops"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
)

type ExecutorClient interface {
	Execute(ctx context.Context, req *graphql.Query) ([]byte, error)
}

// Executor has a map of all the executor clients such that it can execute a
// subquery on any of the federated servers.
// The planner allows it to coordinate the subqueries being sent to the federated servers
type Executor struct {
	Executors map[string]ExecutorClient
	planner   *Planner
}

func fetchSchema(ctx context.Context, e ExecutorClient) ([]byte, error) {
	query, err := graphql.Parse(introspection.IntrospectionQuery, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	return e.Execute(ctx, query)
}

func NewExecutor(ctx context.Context, executors map[string]ExecutorClient) (*Executor, error) {
	// Fetches the schemas from the executors clients
	schemas := make(map[string]*introspectionQueryResult)
	for server, client := range executors {
		schema, err := fetchSchema(ctx, client)
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
	introspectionServer := &Server{schema: introspectionSchema}

	executors["introspection"] = &DirectExecutorClient{Client: introspectionServer}
	schema, err := introspection.RunIntrospectionQuery(introspection.BareIntrospectionSchema(introspectionServer.schema))

	var iq introspectionQueryResult
	if err := json.Unmarshal(schema, &iq); err != nil {
		return nil, oops.Wrapf(err, "unmarshaling introspection schema")
	}

	schemas["introspection"] = &iq
	types, err = convertSchema(schemas)
	if err != nil {
		return nil, oops.Wrapf(err, "converting schemas error")
	}

	flattener, err := newFlattener(types.Schema)
	if err != nil {
		return nil, oops.Wrapf(err, "flattening schemas error")
	}

	// The planner is aware of the merged schema and what executors
	// know about what fields
	planner := &Planner{
		schema:    types,
		flattener: flattener,
	}

	return &Executor{
		Executors: executors,
		planner:   planner,
	}, nil

}
