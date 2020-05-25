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

func getRootGraphqlType(typ graphql.Type) graphql.Type {
	// if typ.(graphql.List) {
	// 	return getRootType(typ.)
	// }
	// swicth typ := typ.(type) {

	// }
	switch typ := typ.(type) {
	case *graphql.List:
		// fmt.Println("LSTTT", typ)
		return getRootGraphqlType(typ.Type)
	case *graphql.NonNull:
		// fmt.Println("NONULL", typ)
		return getRootGraphqlType(typ.Type)
	case *graphql.Object:
		fmt.Println("PMGVHBKJNLKMGJ BHKJNHV GJKNJGHVBKN", typ)
		if typ.Name == "__InputValue" {
			for name, f := range typ.Fields {
				fmt.Println(name, f.Type)
			}
			return typ
		}
		return nil
	default:
		fmt.Println(typ)
		return nil
	}
	// return nil
}

func (e *Executor) runOnService(ctx context.Context, service string, typName string, keys []interface{}, kind string, selectionSet *graphql.SelectionSet) (map[string]interface{}, error) {
	schema := e.Executors[service]

	isRoot := keys == nil
	if !isRoot {
		federatedName := fmt.Sprintf("%s-%s", typName, service)
		newKeys := make(map[string]interface{}, len(keys))

		var rootObject *graphql.Object
		var ok bool
		for a, _ := range e.planner.schema.Fields {
			if a.Type.String() == typName {
				rootObject, ok = a.Type.(*graphql.Object)
				if !ok {
					return nil, oops.Errorf("WRONG")
				}
			}
		}
		for name, key := range keys[0].(map[string]interface{}) {
			if name == "__key" {
				continue
			}
			for fieldName, field := range rootObject.Fields {
				if fieldName == name {
					_, ok := field.FederatedKey[service]
					if ok {
						newKeys[name] = key
						fmt.Println("ADDING", name, key, "TO SERVICE", service)
					}
				}
			}
		}

		selectionSet = &graphql.SelectionSet{
			Selections: []*graphql.Selection{
				{
					Name:  "__federation",
					Alias: "__federation",
					Args:  map[string]interface{}{},
					SelectionSet: &graphql.SelectionSet{
						Selections: []*graphql.Selection{
							{
								Name:  federatedName,
								Alias: federatedName,
								UnparsedArgs: map[string]interface{}{
									"keys": []interface{}{newKeys},
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
	if !isRoot {
		result, ok = result["__federation"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("root did not have a federation map, got %v", res)
		}
		federatedName := fmt.Sprintf("%s-%s", typName, service)
		r, ok := result[federatedName].([]interface{})
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
		key, ok := obj["__federation"]
		if !ok {
			return fmt.Errorf("missing __federation: %v", obj)
		}
		pathTargets.results = obj
		pathTargets.keys = append(pathTargets.keys, key)
		return nil
	}

	obj, ok := node.(map[string]interface{})
	if !ok {
		return nil
	}

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

	case KindType:
		typ, ok := obj["__typename"].(string)
		if !ok {
			return fmt.Errorf("does not have string key __typename")
		}

		if typ == step.Name {
			if err := pathTargets.extractKeys(obj, path[1:]); err != nil {
				return fmt.Errorf("typ %s: %v", typ, err)
			}
		}
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
				return nil, fmt.Errorf("failed to extratc keys %v: %v", subPlan.Path, err)
			}
		}

		g.Go(func() error {
			// Execute the subquery on the specified service
			results, err := e.execute(ctx, subPlan, subPlanMetaData.keys)
			fmt.Println("results", results, err)
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

func deleteKey(v interface{}, k string) {
	switch v := v.(type) {
	case []interface{}:
		for _, e := range v {
			deleteKey(e, k)
		}
	case map[string]interface{}:
		delete(v, k)
		for _, e := range v {
			deleteKey(e, k)
		}
	}
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

	deleteKey(r, "__federation")
	return r, nil
}
