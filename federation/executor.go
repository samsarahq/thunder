package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/samsarahq/go/oops"
	"golang.org/x/sync/errgroup"

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

func (e *Executor) runOnService(ctx context.Context, service string, typName string, keys []interface{}, kind string, selectionSet *graphql.SelectionSet) (map[string]interface{}, error) {
	// Execute query on specified service
	schema, ok := e.Executors[service]
	if !ok {
		return nil, oops.Errorf("service not recognized")
	}
	bytes, err := schema.Execute(ctx, &graphql.Query{
		Kind:         kind,
		SelectionSet: selectionSet,
	})
	if err != nil {
		return nil, oops.Wrapf(err, "execute remotely")
	}
	// Unmarshal json from results
	var res interface{}
	if err := json.Unmarshal(bytes, &res); err != nil {
		return nil, oops.Wrapf(err, "unmarshal res")
	}
	result, ok := res.(map[string]interface{})
	if !ok {
		return nil, oops.Errorf("executor res not a map[string]interface{}")
	}
	return result, nil
}

func (e *Executor) execute(ctx context.Context, p *Plan, keys []interface{}) (interface{}, error) {
	res := map[string]interface{}{}

	// Executes that part of the plan (the subquery) on one of the federated gqlservers
	if p.Service != gatewayCoordinatorServiceName {
		var err error
		res, err = e.runOnService(ctx, p.Service, p.Type, keys, p.Kind, p.SelectionSet)
		if err != nil {
			return nil, oops.Wrapf(err, "run on service")
		}
	}

	g, ctx := errgroup.WithContext(ctx)
	// resMu protects the results (res) as we stitch the results together from seperate goroutines
	// executing in different parts of the plan on different services
	var resMu sync.Mutex

	// For every nested query in the plan, execute it on the specified service and stitch
	// the results into a response
	for _, currentSubPlan := range p.After {
		subPlan := currentSubPlan
		var subPlanMetaData pathSubqueryMetadata
		if p.Service == gatewayCoordinatorServiceName {
			subPlanMetaData.keys = nil // On the root query there are no specified keys
			subPlanMetaData.results = res
		}

		g.Go(func() error {
			// Execute the subquery on the specified service
			results, err := e.execute(ctx, subPlan, subPlanMetaData.keys)
			if err != nil {
				return oops.Wrapf(err, "executing sub plan: %v", err)
			}

			result, ok := results.(map[string]interface{})
			if !ok {
				return fmt.Errorf("result is not an object: %v", result)
			}

			// Acquire mutex lock before modifying results
			resMu.Lock()
			defer resMu.Unlock()
			for k, v := range result {
				subPlanMetaData.results[k] = v
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return res, nil
}

// Metadata for a subquery
type pathSubqueryMetadata struct {
	keys    []interface{}          // Federated Keys passed into subquery
	results map[string]interface{} // Results from subquery
}

func (e *Executor) Execute(ctx context.Context, query *graphql.Query) (interface{}, error) {
	plan, err := e.planner.planRoot(query)
	if err != nil {
		return nil, err
	}

	r, err := e.execute(ctx, plan, nil)
	if err != nil {
		return nil, err
	}

	return r, nil
}
