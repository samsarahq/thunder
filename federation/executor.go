package federation

import (
	"context"
	"encoding/json"
	"fmt"

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

	if keys == nil {
		// Root query
	} else {
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
	if keys == nil {
		return []interface{}{res}, nil
	}

	// otherwise:
	root, ok := res.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("did not get back a map from executor, got %v", res)
	}

	federation, ok := root["__federation"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("root did not have a federation map, got %v", res)
	}

	results, ok := federation[typName].([]interface{})
	if !ok {
		return nil, fmt.Errorf("federation map did not have a %s slice, got %v", typName, res)
	}

	return results, nil
}

type StepKind int

const (
	KindType StepKind = iota
	KindField
)

type PathStep struct {
	Kind StepKind
	Name string
}

type Plan struct {
	PathStep []PathStep
	Service  string
	// XXX: What are we using Type for here again? -- oh, it's for the __federation field...
	Type         string
	SelectionSet *SelectionSet
	After        []*Plan
}

// XXX: have a plan about failed conversions and nils everywhere.

func (e *Executor) Execute(ctx context.Context, p *Plan) (interface{}, error) {
	combined := make(map[string]interface{})

	for _, plan := range p.After {
		res, err := e.execute(ctx, plan, nil)
		if err != nil {
			return nil, err
		}

		for k, v := range res[0].(map[string]interface{}) {
			combined[k] = v
		}
	}

	return combined, nil
}

func (e *Executor) execute(ctx context.Context, p *Plan, keys []interface{}) ([]interface{}, error) {
	res, err := e.runOnService(ctx, p.Service, p.Type, keys, p.SelectionSet)
	if err != nil {
		return nil, fmt.Errorf("run on service: %v", err)
	}

	for _, subPlan := range p.After {
		var targets []map[string]interface{}
		var keys []interface{}

		// DFS to follow path

		// XXX: extract and unit test...
		var search func(node interface{}, path []PathStep) error
		search = func(node interface{}, path []PathStep) error {
			// XXX: encode list flattening in path?
			if slice, ok := node.([]interface{}); ok {
				for i, elem := range slice {
					if err := search(elem, path); err != nil {
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
				targets = append(targets, obj)
				keys = append(keys, key)
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

				if err := search(next, path[1:]); err != nil {
					return fmt.Errorf("elem %s: %v", next, err)
				}

			case KindType:
				typ, ok := obj["__typename"].(string)
				if !ok {
					return fmt.Errorf("does not have string key __typename")
				}

				if typ == step.Name {
					if err := search(obj, path[1:]); err != nil {
						return fmt.Errorf("typ %s: %v", typ, err)
					}
				}
			}

			return nil
		}

		if err := search(res, subPlan.PathStep); err != nil {
			return nil, fmt.Errorf("failed to follow path %v: %v", subPlan.PathStep, err)
		}

		// XXX: don't execute here yet??? i mean we can but why? simpler?????? could go back to root?

		// XXX: go
		results, err := e.execute(ctx, subPlan, keys)
		if err != nil {
			return nil, fmt.Errorf("executing sub plan: %v", err)
		}

		if len(results) != len(targets) {
			return nil, fmt.Errorf("got %d results for %d targets", len(results), len(targets))
		}

		for i, target := range targets {
			result, ok := results[i].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("result is not an object: %v", result)
			}
			for k, v := range result {
				target[k] = v
			}
		}
	}

	return res, nil
}

// todo
// concurrent execution
//
// defer
//
// project. harden APIs
// test malformed inputs
// test incompatible schemas
// test forward/backward schema rollout
// validate incoming queries
//
// clean up types in thunder/graphql, clean up flagging
//
// mutations
//
// failure boundaries, timeouts (?)
//
// XXX: cache queries and plans? (late binding of args?) even better, cache selection sets downstream?
// XXX: precompile queries and query plans???
//
// xxx: schema migrations? moving fields?
//
// dependency sets
