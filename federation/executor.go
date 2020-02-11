package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"golang.org/x/sync/errgroup"
)

type ExecutorClient interface {
	Execute(ctx context.Context, req *graphql.Query) ([]byte, error)
}

type Executor struct {
	Executors map[string]ExecutorClient
	planner *Planner
}


func fetchSchema(ctx context.Context, e ExecutorClient) ([]byte, error) {
	// query, err := graphql.Parse(introspection.IntrospectionQuery, map[string]interface{}{})
	// if err != nil {
	// 	return nil, err
	// }

	query, err := graphql.Parse(introspection.IntrospectionQuery, map[string]interface{}{})
	if err != nil {
		return nil, err
	}




	// if err := graphql.PrepareQuery(context.Background(), graphql.Schema.Query, query.SelectionSet); err != nil {
	// 	return nil, err
	// }

	return e.Execute(ctx, query)


	// if err := graphql.PrepareQuery(context.Background(),	e.Client.Schema.Query, 	e.Client.Schema.SelectionSet); err != nil {
	// 	return nil, err
	// }

	// executor := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	// value, err := executor.Execute(context.Background(), schema.Query, nil, query)
	// if err != nil {
	// 	return nil, err
	// }

	// return e.Execute(ctx, query)
}

func NewExecutor(ctx context.Context, executors map[string]ExecutorClient) (*Executor, error) {
	schemas := make(map[string]*introspectionQueryResult)
	for server, client := range executors {
		schema, err := fetchSchema(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("fetching schema %s: %v", server, err)
		}

		var iq introspectionQueryResult
		if err := json.Unmarshal(schema, &iq); err != nil {
			return nil, fmt.Errorf("unmarshaling schema %s: %v", server, err)
		}

		schemas[server] = &iq
	}

	types, err := convertSchema(schemas)
	if err != nil {
		fmt.Println("UEET")
	}

	introspectionSchema := introspection.BareIntrospectionSchema(types.Schema)
	introspectionServer := &Server{schema: introspectionSchema}

	executors["introspection"] = &DirectExecutorClient{Client: introspectionServer}
	schema, err := introspection.RunIntrospectionQuery(introspection.BareIntrospectionSchema(introspectionServer.schema))


	var iq introspectionQueryResult
	if err := json.Unmarshal(schema, &iq); err != nil {
		return nil, fmt.Errorf("unmarshaling introspection schema: %v", err)
	}

	schemas["introspection"] = &iq
	types, err = convertSchema(schemas)
	if err != nil {
		return nil, err
	}

	flattener, err := newFlattener(types.Schema)
	if err != nil {
		return nil, err
	}

	planner := &Planner{
		schema: types,
		flattener: flattener,
	}
	return &Executor{
		Executors: executors,
		planner: planner,
		// schema:    types,
		// flattener: flattener,
	}, nil


}

func (e *Executor) runOnService(ctx context.Context, service string, typName string, keys []interface{}, kind string, selectionSet *graphql.SelectionSet) ([]interface{}, error) {
	schema := e.Executors[service]

	isRoot := keys == nil
	if !isRoot {
		// XXX: halp
		selectionSet = &graphql.SelectionSet{
			Selections: []*graphql.Selection{
				{
					Name:  "__federation",
					Alias: "__federation",
					Args:  map[string]interface{}{},
					SelectionSet: &graphql.SelectionSet{
						Selections: []*graphql.Selection{
							{
								Name:  typName,
								Alias: typName,
								Args: map[string]interface{}{
									// xxx: do we need to marshal these differently? rely on schema handling of scalars?
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


	// XXX: make sure that if this hangs we're still good?
	bytes, err := schema.Execute(ctx, &graphql.Query{
		Kind:         kind,
		SelectionSet: selectionSet,
	})

	if err != nil {
		return nil, fmt.Errorf("execute remotely: %v", err)
	}

	var res interface{}
	if err := json.Unmarshal(bytes, &res); err != nil {
		return nil, fmt.Errorf("unmarshal res: %v", err)
	}

	// for root:
	var results []interface{}
	if !isRoot {
		root, ok := res.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("did not get back a map from executor, got %v", res)
		}

		federation, ok := root["__federation"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("root did not have a federation map, got %v", res)
		}

		results, ok = federation[typName].([]interface{})
		if !ok {
			return nil, fmt.Errorf("federation map did not have a %s slice, got %v", typName, res)
		}
	} else {
		results = []interface{}{res}
	}

	return results, nil
}

type pathFollower struct {
	targets []map[string]interface{}
	keys    []interface{}
}

func (pf *pathFollower) extractTargets(node interface{}, path []PathStep) error {
	// XXX: encode list flattening in path?
	if slice, ok := node.([]interface{}); ok {
		for i, elem := range slice {
			if err := pf.extractTargets(elem, path); err != nil {
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
		pf.targets = append(pf.targets, obj)
		pf.keys = append(pf.keys, key)
		return nil
	}

	obj, ok := node.(map[string]interface{})
	if !ok {
		// XXX: always do this? only if nullable?
		return nil
	}

	step := path[0]
	switch step.Kind {
	case KindField:
		next, ok := obj[step.Name]
		if !ok {
			return fmt.Errorf("does not have key %s", step.Name)
		}

		if err := pf.extractTargets(next, path[1:]); err != nil {
			return fmt.Errorf("elem %s: %v", next, err)
		}

	case KindType:
		typ, ok := obj["__typename"].(string)
		if !ok {
			return fmt.Errorf("does not have string key __typename")
		}

		if typ == step.Name {
			if err := pf.extractTargets(obj, path[1:]); err != nil {
				return fmt.Errorf("typ %s: %v", typ, err)
			}
		}
	}

	return nil
}


func (e *Executor) execute(ctx context.Context, p *Plan, keys []interface{}) ([]interface{}, error) {
	var res []interface{}

	if p.Service == gatewayCoordinatorServiceName {
		res = []interface{}{
			map[string]interface{}{},
		}
	} else {
		var err error
		res, err = e.runOnService(ctx, p.Service, p.Type, keys, p.Kind, p.SelectionSet)

		if err != nil {
			return nil, fmt.Errorf("run on service: %v", err)
		}
	}

	g, ctx := errgroup.WithContext(ctx)

	var resMu sync.Mutex

	for _, subPlan := range p.After {
		subPlan := subPlan
		var pf pathFollower
		if p.Service != gatewayCoordinatorServiceName {
			if err := pf.extractTargets(res, subPlan.Path); err != nil {
				return nil, fmt.Errorf("failed to follow path %v: %v", subPlan.Path, err)
			}
		} else {
			pf.keys = nil
			pf.targets = []map[string]interface{}{
				res[0].(map[string]interface{}),
			}
		}

		g.Go(func() error {
			// XXX: include the path in the error. might be tricky
			// to reconstruct path unless we include it in results
			// (and leave err nil)
			results, err := e.execute(ctx, subPlan, pf.keys)
			if err != nil {
				return fmt.Errorf("executing sub plan: %v", err)
			}

			if len(results) != len(pf.targets) {
				return fmt.Errorf("got %d results for %d targets", len(results), len(pf.targets))
			}

			resMu.Lock()
			defer resMu.Unlock()

			for i, target := range pf.targets {
				result, ok := results[i].(map[string]interface{})
				if !ok {
					return fmt.Errorf("result is not an object: %v", result)
				}
				for k, v := range result {
					target[k] = v
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
		for _, e := range v {
			deleteKey(e, k)
		}
	}
}

func (e *Executor) Execute(ctx context.Context, q *graphql.Query) (interface{}, error) {
	p, err := e.planner.planRoot(q)
	if err != nil {
		return nil, err
	}

	printPlan(p)

	r, err := e.execute(ctx, p, nil)
	if err != nil {
		return nil, err
	}

	res := r[0]
	deleteKey(res, "__federation")
	return res, nil
}
