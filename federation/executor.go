package federation

import (
	"context"
	"encoding/json"
	"fmt"

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
		return nil, fmt.Errorf("converting schema error: %v", err)
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

// Runs a subquery on a specified service and returns the results
// func (e *Executor) runOnService(ctx context.Context, service string, typName string, keys []interface{}, kind string, selectionSet *graphql.SelectionSet) ([]interface{}, error) {
// 	schema := e.Executors[service]

// 	isRoot := keys == nil
// 	if !isRoot {
// 		selectionSet = &graphql.SelectionSet{
// 			Selections: []*graphql.Selection{
// 				{
// 					Name:  "__federation",
// 					Alias: "__federation",
// 					Args:  map[string]interface{}{},
// 					SelectionSet: &graphql.SelectionSet{
// 						Selections: []*graphql.Selection{
// 							{
// 								Name:  typName,
// 								Alias: typName,
// 								Args: map[string]interface{}{
// 									"keys": keys,
// 								},
// 								SelectionSet: selectionSet,
// 							},
// 						},
// 					},
// 				},
// 			},
// 		}
// 	}

// 	// TODO: make sure that if this hangs we're still good?
// 	bytes, err := schema.Execute(ctx, &graphql.Query{
// 		Kind:         kind,
// 		SelectionSet: selectionSet,
// 	})

// 	if err != nil {
// 		return nil, fmt.Errorf("execute remotely: %v", err)
// 	}

// 	var res interface{}
// 	if err := json.Unmarshal(bytes, &res); err != nil {
// 		return nil, fmt.Errorf("unmarshal res: %v", err)
// 	}

// 	var results []interface{}
// 	if !isRoot {
// 		root, ok := res.(map[string]interface{})
// 		if !ok {
// 			return nil, fmt.Errorf("did not get back a map from executor, got %v", res)
// 		}

// 		federation, ok := root["__federation"].(map[string]interface{})
// 		if !ok {
// 			return nil, fmt.Errorf("root did not have a federation map, got %v", res)
// 		}

// 		results, ok = federation[typName].([]interface{})
// 		if !ok {
// 			return nil, fmt.Errorf("federation map did not have a %s slice, got %v", typName, res)
// 		}
// 	} else {
// 		results = []interface{}{res}
// 	}

// 	return results, nil
// }

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

func (e *Executor) Execute(ctx context.Context, q *graphql.Query) (interface{}, error) {
	return nil, nil

}
