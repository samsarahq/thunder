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
}

func fetchSchema(ctx context.Context, e ExecutorClient) ([]byte, error) {
	query, err := graphql.Parse(introspection.IntrospectionQuery, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	selectionSet, err := marshalPbSelections(query.SelectionSet)
	if err != nil {
		return nil, err
	}

	out, err := e.Execute(ctx, &thunderpb.ExecuteRequest{
		Kind:         thunderpb.ExecuteRequest_QUERY,
		Name:         "introspection",
		SelectionSet: selectionSet,
	})
	if err != nil {
		return nil, err
	}

	return out.Result, nil
}

type GrpcExecutorClient struct {
	Client thunderpb.ExecutorClient
}

func (c *GrpcExecutorClient) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	return c.Client.Execute(ctx, req)
}

type DirectExecutorClient struct {
	Client thunderpb.ExecutorServer
}

func (c *DirectExecutorClient) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	return c.Client.Execute(ctx, req)
}

func NewExecutor(ctx context.Context, executors map[string]ExecutorClient) (*Executor, error) {
	schemas := make(map[string]introspectionQueryResult)

	for server, client := range executors {
		schema, err := fetchSchema(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("fetching schema %s: %v", server, err)
		}

		var iq introspectionQueryResult
		if err := json.Unmarshal(schema, &iq); err != nil {
			return nil, fmt.Errorf("unmarshaling schema %s: %v", server, err)
		}

		schemas[server] = iq
	}

	types, err := convertSchema(schemas, Union)
	if err != nil {
		return nil, err
	}

	introspectionSchema := introspection.BareIntrospectionSchema(types.Schema)
	newServer := &Server{schema: introspectionSchema}

	executors["introspection"] = &DirectExecutorClient{Client: newServer}
	server := "introspection"
	schema, err := introspection.RunIntrospectionQuery(introspection.BareIntrospectionSchema(newServer.schema))

	var iq introspectionQueryResult
	if err := json.Unmarshal(schema, &iq); err != nil {
		return nil, fmt.Errorf("unmarshaling schema %s: %v", server, err)
	}

	schemas[server] = iq
	types, err = convertSchema(schemas, Union)
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

func (e *Executor) runOnService(ctx context.Context, service string, typName string, keys []interface{}, kind thunderpb.ExecuteRequest_Kind, selectionSet *graphql.RawSelectionSet) ([]interface{}, error) {
	schema := e.Executors[service]

	isRoot := keys == nil

	if !isRoot {
		// XXX: halp
		selectionSet = &graphql.RawSelectionSet{
			Selections: []*graphql.RawSelection{
				{
					Name:  "__federation",
					Alias: "__federation",
					Args:  map[string]interface{}{},
					SelectionSet: &graphql.RawSelectionSet{
						Selections: []*graphql.RawSelection{
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
		Kind:         kind,
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
		res, err = ec.e.runOnService(ctx, p.Service, p.Type, keys, p.Kind, p.SelectionSet)
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
			if err := pf.extractTargets(res, subPlan.Path); err != nil {
				return nil, fmt.Errorf("failed to follow path %v: %v", subPlan.Path, err)
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
// test incompatible schemas
//   bad schemas (errors paths in merge)
//   missing __federation
// validate incoming queries
//   run against type checker
//
// tooling for schema management
//   track all schema(s)
//   take least common denominator
//   test adding field
//   test moving field between services
//   live schema updates (while running)
//
// error handling
//   downstream server failure
//   downstream server timeout
//
// union, enum, ... merging
//
// add tracing (hooks?)
// add dependency set hooks
// add caching hooks
//
// support enums
//
// deal with rerunner, reactive.Cache
//
// NICE TO HAVE
//
// maybe: failure boundaries, propagate nil(s)
//
// do something about internal fields (__typename, __federation)
//   track if we added field, if so, remove it from result, otherwise keep it
//
// share flatten between federation/ and graphql/
// limit complexity in flatten
//
// move execution-related fields of graphql.Field (similar to FieldInfo)
// let FieldInfo pick resolver, threads, etc. at runtime (to get rid of batch feature flags)
//
// late-bind arguments in queries, so we can re-use parsed queries
// precompile queries and query plans???
// support @if, @unless
//
// analyze current schema, measure number of package transitions / expected plan depth(s)
//
// @defer
//
// swap out websocket implementation to hit HTTP paths
//
// simplify executor, schema, and HTTP handler APIs
//
// extract out dependency set client and server
