package federation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/samsarahq/thunder/graphql/introspection"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/thunderpb"
)

type Executor struct {
	Executors           map[string]ExecutorClient
	IntrospectionSchema *graphql.Schema

	schema SchemaWithFederationInfo
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
	schemas := make(map[string]IntrospectionQuery)

	for server, client := range executors {
		schema, err := client.Schema(ctx, &thunderpb.SchemaRequest{})
		if err != nil {
			return nil, fmt.Errorf("fetching schema %s: %v", server, err)
		}

		var iq IntrospectionQuery
		if err := json.Unmarshal(schema.Schema, &iq); err != nil {
			return nil, fmt.Errorf("unmarshaling schema %s: %v", server, err)
		}

		schemas[server] = iq
	}

	types := convertSchema(schemas)

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

	var iq IntrospectionQuery
	if err := json.Unmarshal(schema.Schema, &iq); err != nil {
		return nil, fmt.Errorf("unmarshaling schema %s: %v", server, err)
	}

	schemas[server] = iq
	types = convertSchema(schemas)

	return &Executor{
		Executors:           executors,
		schema:              types,
		IntrospectionSchema: introspectionSchema,
	}, nil
}

// oooh.. maybe we need to support introspection at the aggregator level... :)

type FieldInfo struct {
	Service  string
	Services map[string]bool
}

type SchemaWithFederationInfo struct {
	Schema *graphql.Schema
	Fields map[*graphql.Field]*FieldInfo
}

type Selection struct {
	Alias      string
	Name       string
	Args       map[string]interface{}
	Selections []*Selection
}

func convertSelectionSet(selections []*Selection) *graphql.RawSelectionSet {
	if len(selections) == 0 {
		return nil
	}

	newSelections := make([]*graphql.RawSelection, 0, len(selections))

	for _, selection := range selections {
		newSelections = append(newSelections, &graphql.RawSelection{
			Alias:        selection.Alias,
			Args:         selection.Args,
			Name:         selection.Name,
			SelectionSet: convertSelectionSet(selection.Selections),
		})
	}

	return &graphql.RawSelectionSet{
		Selections: newSelections,
	}
}

func (e *Executor) runOnService(ctx context.Context, service string, typName string, keys []interface{}, selections []*Selection) ([]interface{}, error) {
	schema := e.Executors[service]

	if keys == nil {
		// Root query
	} else {
		// XXX: halp
		selections = []*Selection{
			{
				Name:  "__federation",
				Alias: "__federation",
				Args:  map[string]interface{}{},
				Selections: []*Selection{
					{
						Name:  typName,
						Alias: typName,
						Args: map[string]interface{}{
							// xxx: do we need to marshal these differently? rely on schema handling of scalars?
							"keys": keys,
						},
						Selections: selections,
					},
				},
			},
		}
	}

	marshaled, err := marshalPbSelections(selections)
	if err != nil {
		return nil, fmt.Errorf("marshaling selections: %v", err)
	}

	resPb, err := schema.Execute(ctx, &thunderpb.ExecuteRequest{
		Selections: marshaled,
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

type Plan struct {
	Path       []string
	Service    string
	Type       string
	Selections []*Selection
	After      []*Plan
}

// XXX: have a plan about failed conversions and nils everywhere.

func (e *Executor) Execute(ctx context.Context, p *Plan, keys []interface{}) ([]interface{}, error) {
	res, err := e.runOnService(ctx, p.Service, p.Type, keys, p.Selections)
	if err != nil {
		return nil, fmt.Errorf("run on service: %v", err)
	}

	for _, subPlan := range p.After {
		var targets []map[string]interface{}
		var keys []interface{}

		// targets = []interface{}{res}

		// DFS to follow path

		// XXX: extract and unit test...
		var search func(node interface{}, path []string) error
		search = func(node interface{}, path []string) error {
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

			next, ok := obj[path[0]]
			if !ok {
				return fmt.Errorf("does not have key %s", path[0])
			}

			if err := search(next, path[1:]); err != nil {
				return fmt.Errorf("elem %s: %v", next, err)
			}

			return nil
		}

		if err := search(res, subPlan.Path); err != nil {
			return nil, fmt.Errorf("failed to follow path %v: %v", subPlan.Path, err)
		}

		// XXX: don't execute here yet??? i mean we can but why? simpler?????? could go back to root?

		// XXX: go
		results, err := e.Execute(ctx, subPlan, keys)
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

func (e *Executor) plan(typ *graphql.Object, selections []*Selection, service string) (*Plan, error) {
	p := &Plan{
		Type:       typ.Name,
		Service:    service,
		Selections: nil,
		After:      nil,
	}

	// XXX: pass in sub-path (and sub-plan slice?) to make sub-plan munging simpler?

	// - should we type check here?
	// - what do we here with fragments? inline them?
	// - the executor might need to know the types to switch?

	// - refactor to return array of subplans with preferential treatment for given service?
	// eh whatever.

	/*
		switch "" {
		case "union":
			// add branch for every possible type

		case "object":
			// ...

		case "":
			return nil
		}
	*/

	selectionsByService := make(map[string][]*Selection)

	for _, selection := range selections {
		field, ok := typ.Fields[selection.Name]
		if !ok {
			return nil, fmt.Errorf("typ %s has no field %s", typ.Name, selection.Name)
		}

		fieldInfo := e.schema.Fields[field]

		// if we can stick to the current service, stay there
		if fieldInfo.Services[service] {
			selectionsByService[service] = append(
				selectionsByService[service], selection)
		} else {
			selectionsByService[fieldInfo.Service] = append(
				selectionsByService[fieldInfo.Service], selection)
		}
	}

	// spew.Dump(service, selectionsByService)

	// if we encounter a fragment, we find a branch
	// either we hit it, or we don't. must make plan for both cases?
	//
	// very snazzily we could merge subplans.

	for _, selection := range selectionsByService[service] {
		// we have already checked above that this field exists
		field := typ.Fields[selection.Name]

		var childPlan *Plan
		if selection.Selections != nil {
			// XXX: assert existence of types elsewhere?
			var err error
			// XXX type assertoin
			childPlan, err = e.plan(field.Type.(*graphql.Object), selection.Selections, service)
			if err != nil {
				return nil, fmt.Errorf("planning for %s: %v", selection.Name, err)
			}
		}

		newSelection := &Selection{
			Alias: selection.Alias,
			Name:  selection.Name,
			Args:  selection.Args,
		}
		if childPlan != nil {
			newSelection.Selections = childPlan.Selections
		}

		p.Selections = append(p.Selections, newSelection)

		if childPlan != nil {
			for _, subPlan := range childPlan.After {
				subPlan.Path = append([]string{selection.Alias}, subPlan.Path...)
				p.After = append(p.After, subPlan)
			}
		}
	}

	needKey := false

	for other, selections := range selectionsByService {
		if other == service {
			continue
		}
		needKey = true

		// what if a field has multiple options? should we consider capacity?
		// what other fields we might want to resolve after?
		// nah, just go with default... and consider being able to stick with
		// the same a bonus
		subPlan, err := e.plan(typ, selections, other)
		if err != nil {
			return nil, fmt.Errorf("planning for %s: %v", other, err)
		}

		p.After = append(p.After, subPlan)
	}

	if needKey {
		hasKey := false
		for _, selection := range p.Selections {
			if selection.Name == "__federation" && selection.Alias == "__federation" {
				hasKey = true
			} else if selection.Alias == "__federation" {
				// error, conflict, can't do this.
			}
		}
		if !hasKey {
			p.Selections = append(p.Selections, &Selection{
				Name:  "__federation",
				Alias: "__federation",
				Args:  map[string]interface{}{},
			})
		}
	}

	return p, nil
}

func (e *Executor) Plan(query *graphql.RawSelectionSet) (*Plan, error) {
	return e.plan(e.schema.Schema.Query.(*graphql.Object), convert(query), "no-such-service")
}

func convert(query *graphql.RawSelectionSet) []*Selection {
	if query == nil {
		return nil
	}

	var converted []*Selection
	for _, selection := range query.Selections {
		if selection.Alias == "" {
			selection.Alias = selection.Name
		}
		converted = append(converted, &Selection{
			Name:       selection.Name,
			Alias:      selection.Alias,
			Args:       selection.Args,
			Selections: convert(selection.SelectionSet),
		})
		// XXX: janky hack
	}
	for _, fragment := range query.Fragments {
		converted = append(converted, convert(fragment.SelectionSet)...)
	}

	return converted
}

// todo
// project. expose introspection query
// federate onto introspectoin server (!?!?!)
// serve graphiql
//
// project. harden APIs
// test malformed inputs
// test incompatible schemas
// test forward/backward schema rollout
// multiple root fields
// validate incoming queries
//
// project. fragments
//
// project. union types
//
// clean up types in thunder/graphql, clean up flagging
//
// mutations
//
// defer
//
// failure boundaries, timeouts (?)
//
// XXX: cache queries and plans? even better, cache selection sets downstream?
// XXX: precompile queries and query plans???
//
// xxx: concurrent execution
//
// xxx: schema migrations? moving fields?
//
// dependency sets
