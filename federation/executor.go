package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"

	"github.com/davecgh/go-spew/spew"
	"google.golang.org/grpc"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/thunderpb"
)

type Executor struct {
	Executors map[string]thunderpb.ExecutorClient

	schema SchemaWithFederationInfo
}

func NewExecutor(ctx context.Context, executors map[string]thunderpb.ExecutorClient) (*Executor, error) {
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
	return &Executor{
		Executors: executors,
		schema:    types,
	}, nil
}

// oooh.. maybe we need to support introspection at the aggregator level... :)

type FieldInfo struct {
	Service  string
	Services map[string]bool
}

type SchemaWithFederationInfo struct {
	Query  *graphql.Object
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

	// xxx: detect if root (?)

	if keys == nil {
		// Root query
		// selectionSet = convertSelectionSet(selections)
	} else {
		var garbage interface{}
		bytes, err := json.Marshal(keys)
		if err != nil {
			return nil, fmt.Errorf("roundtripping keys: %v", err)
		}
		if err := json.Unmarshal(bytes, &garbage); err != nil {
			return nil, fmt.Errorf("roudntripping keys: %v", err)
		}

		// XXX: halp
		selections = []*Selection{
			{
				Name:  typName + "sFromFederationKeys",
				Alias: "results",
				Args: map[string]interface{}{
					"keys": garbage, // keys,
				},
				Selections: selections,
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
	asMap, ok := res.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("did not get back a map from executor, got %v", res)
	}

	results, ok := asMap["results"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("map did not have a results slice, got %v", res)
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

func (e *Executor) execute(ctx context.Context, p *Plan, keys []interface{}) ([]interface{}, error) {
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
				key, ok := obj["federationKey"]
				if !ok {
					return fmt.Errorf("missing federationKey: %v", obj)
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
			if selection.Name == "federationKey" && selection.Alias == "federationKey" {
				hasKey = true
			} else if selection.Alias == "federationKey" {
				// error, conflict, can't do this.
			}
		}
		if !hasKey {
			p.Selections = append(p.Selections, &Selection{
				Name:  "federationKey",
				Alias: "federationKey",
				Args:  map[string]interface{}{},
			})
		}
	}

	return p, nil
}

func (e *Executor) Plan(typ *graphql.Object, selections []*Selection) (*Plan, error) {
	return e.plan(typ, selections, "no-such-service")
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
	}
	return converted
}

func makeExecutors(schemas map[string]*schemabuilder.Schema) (_ map[string]thunderpb.ExecutorClient, close func(), err error) {
	var closers []func()
	defer func() {
		if err != nil {
			for _, close := range closers {
				close()
			}
		}
	}()

	executors := make(map[string]thunderpb.ExecutorClient)

	for name, schema := range schemas {
		srv, err := NewServer(schema)
		if err != nil {
			return nil, nil, err
		}

		grpcServer := grpc.NewServer()
		thunderpb.RegisterExecutorServer(grpcServer, srv)

		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, nil, err
		}

		go grpcServer.Serve(listener)

		closers = append(closers, func() {
			listener.Close()
			grpcServer.Stop()
		})

		client, err := grpc.Dial(listener.Addr().String(), grpc.WithInsecure())
		if err != nil {
			return nil, nil, err
		}

		closers = append(closers, func() {
			client.Close()
		})

		executors[name] = thunderpb.NewExecutorClient(client)
	}

	return executors, func() {
		for _, close := range closers {
			close()
		}
	}, nil
}

func mustExtractSchema(schema *schemabuilder.Schema) IntrospectionQuery {
	bytes, err := introspection.ComputeSchemaJSON(*schema)
	if err != nil {
		log.Fatal(err)
	}
	var iq IntrospectionQuery
	if err := json.Unmarshal(bytes, &iq); err != nil {
		log.Fatal(err)
	}
	return iq
}

func mustExtractSchemas(schemas map[string]*schemabuilder.Schema) map[string]IntrospectionQuery {
	out := make(map[string]IntrospectionQuery)
	for k, v := range schemas {
		out[k] = mustExtractSchema(v)
	}
	return out
}

func main() {
	ctx := context.Background()

	execs, _, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": schema1(),
		"schema2": schema2(),
	})
	if err != nil {
		log.Fatal(err)
	}

	e, err := NewExecutor(ctx, execs)
	if err != nil {
		log.Fatal(err)
	}

	oldQuery := graphql.MustParse(`
		{
			fff {
				a: nest { b: nest { c: nest { ok } } }
				hmm
				ok
				bar {
					id
					baz
				}
			}
		}
	`, map[string]interface{}{})

	query := convert(oldQuery.SelectionSet)

	plan, err := e.Plan(e.schema.Query, query)
	if err != nil {
		log.Fatal(err)
	}

	// XXX: have to deal with multiple plans here
	res, err := e.execute(ctx, plan.After[0], nil)
	if err != nil {
		log.Fatal(err)
	}

	spew.Dump(res)
}

// todo
// project. schema api
// basic sketch with two packages
//
// project. federation API
// clean up xyzFromFederationKeys
//
// project. harden APIs
// test malformed inputs
// test incompatible schemas
// test forward/backward schema rollout
// validate incoming queries
//
// project. expose introspection query
// federate onto introspectoin server (!?!?!)
//
// project. fragments
//
// project. union types
//
// project. independnet binaries
//
// XXX: cache queries and plans? even better, cache selection sets downstream?
// XXX: precompile queries and query plans???
