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

const keyField = "__key"
const federationField = "__federation"

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
	schema, ok := e.Executors[service]
	if !ok {
		return nil, oops.Errorf("service not recognized")
	}

	// If it is not a root query, nest the subquery on the federation field
	// and pass the keys in to find the object that the subquery is nested on
	// {
	//    __federation {
	//     [ObjectName] (keys: Keys) {
	//       subQuery
	// 		}
	//   }
	// }
	isRoot := keys == nil
	if !isRoot {
		selectionSet = &graphql.SelectionSet{
			Selections: []*graphql.Selection{
				{
					Name:  federationField,
					Alias: federationField,
					Args:  map[string]interface{}{},
					SelectionSet: &graphql.SelectionSet{
						Selections: []*graphql.Selection{
							{
								Name:  typName,
								Alias: typName,
								UnparsedArgs: map[string]interface{}{
									"keys": keys,
								},
								SelectionSet: selectionSet,
							},
						},
					},
				},
			},
		}
	}

	// Execute query on specified service
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
	if !isRoot {
		result, ok = result[federationField].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("root did not have a federation map, got %v", res)
		}

		r, ok := result[typName].([]interface{})
		if !ok {
			return nil, fmt.Errorf("federation map did not have a %s slice, got %v", typName, res)
		}

		if len(r) != 1 {
			return nil, fmt.Errorf("federation had incorect number of results for %s slice, got %v", typName, res)
		}

		res, ok := r[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("federation map did not have an element in %s slice, got %v", typName, res)
		}
		return res, nil

	}
	return result, nil
}

func (pathTargets *pathSubqueryMetadata) extractKeys(node interface{}, path []PathStep) error {
	// Extract key for every element in the slice
	if slice, ok := node.([]interface{}); ok {
		for i, elem := range slice {
			if err := pathTargets.extractKeys(elem, path); err != nil {
				return fmt.Errorf("idx %d: %v", i, err)
			}
		}
		return nil
	}

	if len(path) == 0 {
		obj, ok := node.(map[string]interface{})
		if !ok {
			return fmt.Errorf("not an object: %v", obj)
		}
		key, ok := obj[federationField]
		if !ok {
			return fmt.Errorf("missing __federation: %v", obj)
		}
		// Add a pointer to the object for where the results from
		// the subquery will be added into the final result
		pathTargets.results = obj
		// Keys from the "__federation" field func are passed to
		// the subquery
		pathTargets.keys = append(pathTargets.keys, key)
		return nil
	}

	obj, ok := node.(map[string]interface{})
	if !ok {
		return nil
	}

	// Extract keys nested on the object
	step := path[0]
	switch step.Kind {
	case KindField:
		next, ok := obj[step.Name]
		if !ok {
			return fmt.Errorf("does not have key %s", step.Name)
		}
		if err := pathTargets.extractKeys(next, path[1:]); err != nil {
			return fmt.Errorf("elem %s: %v", next, err)
		}
	default:
		return fmt.Errorf("unsupported step type name: %s kind: %v", step.Name, step.Kind)
	}

	return nil
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
		} else {
			if err := subPlanMetaData.extractKeys(res, subPlan.Path); err != nil {
				return nil, fmt.Errorf("failed to extract keys %v: %v", subPlan.Path, err)
			}
		}

		g.Go(func() error {
			// Execute the subquery on the specified service
			results, err := e.execute(ctx, subPlan, subPlanMetaData.keys)
			if err != nil {
				return oops.Wrapf(err, "executing sub plan: %v", err)
			}

			result, ok := results.(map[string]interface{})
			if !ok {
				return oops.Errorf("result is not an object: %v", result)
			}

			// Acquire mutex lock before modifying results
			resMu.Lock()
			defer resMu.Unlock()
			for k, v := range result {
				if _, ok := subPlanMetaData.results[k]; !ok {
					deleteKey(v, federationField)
					subPlanMetaData.results[k] = v
				} else {
					if k != keyField || v != subPlanMetaData.results[k] {
						return oops.Errorf("key already exists in results: %v", k)
					}
				}

			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return res, nil
}

func deleteKey(v interface{}, k string) {
	switch v := v.(type) {
	case []interface{}:
		for _, e := range v {
			deleteKey(e, k)
		}
	case map[string]interface{}:
		delete(v, k)
	}
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
