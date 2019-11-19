package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/thunderpb"
)

type Executor struct {
	Executors           map[string]ExecutorClient
	IntrospectionSchema *graphql.Schema

	schema *SchemaWithFederationInfo
	types  map[string]graphql.Type
}

type ExecutorClient interface {
	Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error)
	Schema(ctx context.Context, req *thunderpb.SchemaRequest) (*thunderpb.SchemaResponse, error)
}

type GrpcExecutorClient struct {
	Client thunderpb.ExecutorClient
}

func (c *GrpcExecutorClient) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	return c.Client.Execute(ctx, req)
}

func (c *GrpcExecutorClient) Schema(ctx context.Context, req *thunderpb.SchemaRequest) (*thunderpb.SchemaResponse, error) {
	return c.Client.Schema(ctx, req)
}

type DirectExecutorClient struct {
	Client thunderpb.ExecutorServer
}

func (c *DirectExecutorClient) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	return c.Client.Execute(ctx, req)
}

func (c *DirectExecutorClient) Schema(ctx context.Context, req *thunderpb.SchemaRequest) (*thunderpb.SchemaResponse, error) {
	return c.Client.Schema(ctx, req)
}

func NewExecutor(ctx context.Context, executors map[string]ExecutorClient) (*Executor, error) {
	schemas := make(map[string]introspectionQueryResult)

	for server, client := range executors {
		schema, err := client.Schema(ctx, &thunderpb.SchemaRequest{})
		if err != nil {
			return nil, fmt.Errorf("fetching schema %s: %v", server, err)
		}

		var iq introspectionQueryResult
		if err := json.Unmarshal(schema.Schema, &iq); err != nil {
			return nil, fmt.Errorf("unmarshaling schema %s: %v", server, err)
		}

		schemas[server] = iq
	}

	types, err := convertSchema(schemas)
	if err != nil {
		return nil, err
	}

	introspectionSchema := introspection.BareIntrospectionSchema(types.Schema)
	newServer, err := NewServer(introspectionSchema)
	if err != nil {
		return nil, err
	}

	executors["introspection"] = &DirectExecutorClient{Client: newServer}
	server := "introspection"
	client := executors[server]
	schema, err := client.Schema(ctx, &thunderpb.SchemaRequest{})
	if err != nil {
		return nil, fmt.Errorf("fetching schema %s: %v", server, err)
	}

	var iq introspectionQueryResult
	if err := json.Unmarshal(schema.Schema, &iq); err != nil {
		return nil, fmt.Errorf("unmarshaling schema %s: %v", server, err)
	}

	schemas[server] = iq
	types, err = convertSchema(schemas)
	if err != nil {
		return nil, err
	}

	allTypes := make(map[graphql.Type]string)
	if err := collectTypes(types.Schema.Query, allTypes); err != nil {
		return nil, err
	}
	reversedTypes := make(map[string]graphql.Type)
	for typ, name := range allTypes {
		reversedTypes[name] = typ
	}

	return &Executor{
		Executors:           executors,
		schema:              types,
		IntrospectionSchema: introspectionSchema,
		types:               reversedTypes,
	}, nil
}

func (e *Executor) runOnService(ctx context.Context, service string, typName string, keys []interface{}, selectionSet *SelectionSet) ([]interface{}, error) {
	schema := e.Executors[service]

	isRoot := keys == nil

	if !isRoot {
		// XXX: halp
		selectionSet = &SelectionSet{
			Selections: []*Selection{
				{
					Name:  "__federation",
					Alias: "__federation",
					Args:  map[string]interface{}{},
					SelectionSet: &SelectionSet{
						Selections: []*Selection{
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

	marshaled, err := marshalPbSelections(selectionSet)
	if err != nil {
		return nil, fmt.Errorf("marshaling selections: %v", err)
	}

	resPb, err := schema.Execute(ctx, &thunderpb.ExecuteRequest{
		SelectionSet: marshaled,
	})
	if err != nil {
		return nil, fmt.Errorf("execute remotely: %v", err)
	}

	var res interface{}
	if err := json.Unmarshal(resPb.Result, &res); err != nil {
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

// XXX: have a plan about failed conversions and nils everywhere.

type pathFollower struct {
	targets []map[string]interface{}
	keys    []interface{}
}

// XXX: this needs some tests
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

type executorContext struct {
	e *Executor

	outputMu sync.Mutex
	wg       sync.WaitGroup
	err      error
}

func (ec *executorContext) setError(err error) {
	// XXX: test
	if ec.err == nil {
		ec.err = err
	}
}

func (ec *executorContext) execute(ctx context.Context, p *Plan, keys []interface{}) ([]interface{}, error) {
	var res []interface{}
	if p.Service != "no-such-service" {
		var err error
		res, err = ec.e.runOnService(ctx, p.Service, p.Type, keys, p.SelectionSet)
		if err != nil {
			return nil, fmt.Errorf("run on service: %v", err)
		}
	} else {
		res = []interface{}{
			map[string]interface{}{},
		}
	}

	for _, subPlan := range p.After {
		subPlan := subPlan
		var pf pathFollower
		if p.Service != "no-such-service" {
			if err := pf.extractTargets(res, subPlan.PathStep); err != nil {
				return nil, fmt.Errorf("failed to follow path %v: %v", subPlan.PathStep, err)
			}
		} else {
			pf.keys = nil
			pf.targets = []map[string]interface{}{
				res[0].(map[string]interface{}),
			}
		}

		// XXX: go
		ec.wg.Add(1)
		go func() {
			defer ec.wg.Done()

			results, err := ec.execute(ctx, subPlan, pf.keys)

			ec.outputMu.Lock()
			defer ec.outputMu.Unlock()

			if err != nil {
				ec.setError(fmt.Errorf("executing sub plan: %v", err))
				return
			}

			if len(results) != len(pf.targets) {
				ec.setError(fmt.Errorf("got %d results for %d targets", len(results), len(pf.targets)))
				return
			}

			for i, target := range pf.targets {
				result, ok := results[i].(map[string]interface{})
				if !ok {
					ec.setError(fmt.Errorf("result is not an object: %v", result))
					return
				}
				for k, v := range result {
					target[k] = v
				}
			}
		}()
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

func (e *Executor) Execute(ctx context.Context, p *Plan) (interface{}, error) {
	ec := executorContext{
		e: e,
	}

	r, err := ec.execute(ctx, p, nil)
	if err != nil {
		return nil, err
	}
	ec.wg.Wait()
	if ec.err != nil {
		return nil, ec.err
	}

	res := r[0]
	deleteKey(res, "__federation")
	return res, nil
}

// todo
//
// NEEDED
//
// mutations
//
// http handler
//
// project. harden APIs
// test malformed inputs
// test incompatible schemas
// test forward/backward schema rollout
// validate incoming queries
// xxx: schema migrations? moving fields?
// do something about internal fields (__typename, __federation)
//
// tooling for schema management
//
// dependency sets
// caching?
// tracing (hooks?)
//
// NICE TO HAVE
//
// use same types in federation/ and graphql/
//
// share flatten between federation/ and graphql/
//
// limit complexity in flatten
//
// clean up types in thunder/graphql, clean up flagging
//
// XXX: cache queries and plans? (late binding of args?) even better, cache selection sets downstream?
// XXX: precompile queries and query plans???
// if, unless
//
// analyze current schema, measure number of package transitions / expected plan depth(s)
//
// failure boundaries, timeouts (?)
//
// defer
//
// - swap out websocket implementation to hit HTTP paths