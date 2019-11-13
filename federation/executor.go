package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/thunderpb"
)

type Executor struct {
	Executors           map[string]ExecutorClient
	IntrospectionSchema *graphql.Schema

	schema *SchemaWithFederationInfo
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

	var iq IntrospectionQuery
	if err := json.Unmarshal(schema.Schema, &iq); err != nil {
		return nil, fmt.Errorf("unmarshaling schema %s: %v", server, err)
	}

	schemas[server] = iq
	types, err = convertSchema(schemas)
	if err != nil {
		return nil, err
	}

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
	Alias string
	Name  string
	Args  map[string]interface{}
	// Selections []*Selection
	SelectionSet *SelectionSet
}

type Fragment struct {
	On           string
	SelectionSet *SelectionSet
}

type SelectionSet struct {
	Selections []*Selection
	Fragments  []*Fragment
}

func convertSelectionSet(selectionSet *SelectionSet) *graphql.RawSelectionSet {
	if selectionSet == nil {
		return nil
	}

	newSelections := make([]*graphql.RawSelection, 0, len(selectionSet.Selections))

	for _, selection := range selectionSet.Selections {
		newSelections = append(newSelections, &graphql.RawSelection{
			Alias:        selection.Alias,
			Args:         selection.Args,
			Name:         selection.Name,
			SelectionSet: convertSelectionSet(selection.SelectionSet),
		})
	}

	fragments := make([]*graphql.RawFragment, 0, len(selectionSet.Fragments))

	for _, fragment := range selectionSet.Fragments {
		fragments = append(fragments, &graphql.RawFragment{
			On:           fragment.On,
			SelectionSet: convertSelectionSet(fragment.SelectionSet),
		})
	}

	return &graphql.RawSelectionSet{
		Selections: newSelections,
		Fragments:  fragments,
	}
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

type AfterNodeKind int

const (
	KindType  = 1
	KindField = 2
)

type AfterNode struct {
	Kind     AfterNodeKind
	Next     map[string]*AfterNode
	Services map[string]*Plan
}

type PathStep struct {
	Kind AfterNodeKind
	Name string
}

type Plan struct {
	// Path    []string
	PathStep []PathStep
	Service  string
	// XXX: What are we using Type for here again?
	Type         string
	SelectionSet *SelectionSet
	After        []*Plan
	// AfterNode *AfterNode
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

		// targets = []interface{}{res}

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

func applies(obj *graphql.Object, fragment *Fragment, allTypes map[string]graphql.Type) (bool, error) {
	switch typ := allTypes[fragment.On].(type) {
	case *graphql.Object:
		return typ.Name == obj.Name, nil

	case *graphql.Union:
		_, ok := typ.Types[obj.Name]
		return ok, nil

	default:
		return false, fmt.Errorf("bad fragment %v", fragment.On)
	}
}

func collectTypes(typ graphql.Type, types map[graphql.Type]string) error {
	if _, ok := types[typ]; ok {
		return nil
	}

	switch typ := typ.(type) {
	case *graphql.NonNull:
		collectTypes(typ.Type, types)

	case *graphql.List:
		collectTypes(typ.Type, types)

	case *graphql.Object:
		types[typ] = typ.Name

		for _, field := range typ.Fields {
			collectTypes(field.Type, types)
		}

	case *graphql.Union:
		types[typ] = typ.Name
		for _, obj := range typ.Types {
			collectTypes(obj, types)
		}

	case *graphql.Enum:
		types[typ] = typ.Type

	case *graphql.Scalar:
		types[typ] = typ.Type

	default:
		return fmt.Errorf("bad typ %v", typ)
	}

	return nil
}

func flatten(selectionSet *SelectionSet, typ graphql.Type, allTypes map[string]graphql.Type) (*SelectionSet, error) {
	switch typ := typ.(type) {
	case *graphql.NonNull:
		return flatten(selectionSet, typ.Type, allTypes)

	case *graphql.List:
		return flatten(selectionSet, typ.Type, allTypes)

	case *graphql.Object:
		// XXX: type check?
		selections := make([]*Selection, len(selectionSet.Selections))
		copy(selections, selectionSet.Selections)

		for _, fragment := range selectionSet.Fragments {
			ok, err := applies(typ, fragment, allTypes)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}

			flattened, err := flatten(fragment.SelectionSet, typ, allTypes)
			if err != nil {
				return nil, err
			}

			selections = append(selections, flattened.Selections...)
		}

		return &SelectionSet{
			Selections: selections,
		}, nil

		// great

	case *graphql.Union:
		// XXX: all these selections must be on on __typename. type check?
		selections := make([]*Selection, len(selectionSet.Selections))
		copy(selections, selectionSet.Selections)

		fragments := make([]*Fragment, 0, len(typ.Types))
		for _, obj := range typ.Types {
			plan, err := flatten(selectionSet, typ, allTypes)
			if err != nil {
				return nil, err
			}
			fragments = append(fragments, &Fragment{
				On:           obj.Name,
				SelectionSet: plan,
			})
		}

		return &SelectionSet{
			Selections: selections,
			Fragments:  fragments,
		}, nil

	case *graphql.Enum, *graphql.Scalar:
		// XXX: ensure nil?
		return selectionSet, nil

	default:
		return nil, fmt.Errorf("bad typ %v", typ)
	}
}
func (e *Executor) planObject(typ *graphql.Object, selectionSet *SelectionSet, service string) (*Plan, error) {
	// needTypename := true

	p := &Plan{
		Type:         typ.Name,
		Service:      service,
		SelectionSet: &SelectionSet{},
		After:        nil,
		/*
			AfterNode: &AfterNode{
				Kind:     KindField,
				Next:     map[string]*AfterNode{},
				Services: map[string]*Plan{},
			},
		*/
	}

	// XXX: pass in sub-path (and sub-plan slice?) to make sub-plan munging simpler?

	// - should we type check here?
	// - what do we here with fragments? inline them?
	// - the executor might need to know the types to switch?

	// - refactor to return array of subplans with preferential treatment for given service?
	// eh whatever.

	selectionsByService := make(map[string][]*Selection)

	// A flattened selection set on an object will have only selectoins, no fragments.
	// XXX: assert?

	for _, selection := range selectionSet.Selections {
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
		if selection.SelectionSet != nil {
			// XXX: assert existence of types elsewhere?
			var err error
			// XXX type assertoin
			childPlan, err = e.plan(field.Type, selection.SelectionSet, service)
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
			newSelection.SelectionSet = childPlan.SelectionSet
		}

		p.SelectionSet.Selections = append(p.SelectionSet.Selections, newSelection)

		if childPlan != nil {
			for _, subPlan := range childPlan.After {
				subPlan.PathStep = append(subPlan.PathStep, PathStep{Kind: KindField, Name: selection.Alias})
				p.After = append(p.After, subPlan)
			}
		}
	}

	needKey := false

	var otherServices []string
	for other := range selectionsByService {
		if other != service {
			otherServices = append(otherServices, other)
		}
	}
	sort.Strings(otherServices)

	for _, other := range otherServices {
		selections := selectionsByService[other]
		needKey = true

		// what if a field has multiple options? should we consider capacity?
		// what other fields we might want to resolve after?
		// nah, just go with default... and consider being able to stick with
		// the same a bonus
		subPlan, err := e.plan(typ, &SelectionSet{Selections: selections}, other)
		if err != nil {
			return nil, fmt.Errorf("planning for %s: %v", other, err)
		}

		// p.AfterNode.Services[other] = subPlan
		p.After = append(p.After, subPlan)
	}

	if needKey {
		hasKey := false
		for _, selection := range p.SelectionSet.Selections {
			if selection.Name == "__federation" && selection.Alias == "__federation" {
				hasKey = true
			} else if selection.Alias == "__federation" {
				// error, conflict, can't do this.
			}
		}
		if !hasKey {
			p.SelectionSet.Selections = append(p.SelectionSet.Selections, &Selection{
				Name:  "__federation",
				Alias: "__federation",
				Args:  map[string]interface{}{},
			})
		}
	}

	return p, nil

}

func (e *Executor) planUnion(typ *graphql.Union, selectionSet *SelectionSet, service string) (*Plan, error) {
	// needTypename := true

	overallP := &Plan{
		SelectionSet: &SelectionSet{
			Selections: []*Selection{
				{
					Name:  "__typename",
					Alias: "__typename",
					Args:  map[string]interface{}{},
				},
			},
		},
	}

	/*
		overallP.AfterNode = &AfterNode{
			Kind:     KindType,
			Next:     map[string]*AfterNode{},
			Services: map[string]*Plan{},
		}
	*/

	overallP.SelectionSet.Selections = append(overallP.SelectionSet.Selections, selectionSet.Selections...)

	// fragments := make(map[string]...)

	for _, fragment := range selectionSet.Fragments {
		// This is a flattened selection set.
		typ, ok := typ.Types[fragment.On]
		if !ok {
			return nil, fmt.Errorf("unexpected fragment on %s for typ %s", fragment.On, typ.Name)
		}

		p, err := e.plan(typ, fragment.SelectionSet, service)
		if err != nil {
			return nil, err
		}

		overallP.SelectionSet.Fragments = append(overallP.SelectionSet.Fragments, &Fragment{
			On:           typ.Name,
			SelectionSet: p.SelectionSet,
		})

		for _, subPlan := range p.After {
			// take p.After and make them conditional on __typename == X -- stick that in path? seems nice.
			// XXX: include type in path
			subPlan.PathStep = append(subPlan.PathStep, PathStep{Kind: KindType, Name: typ.Name})
			overallP.After = append(overallP.After, subPlan)
		}

		// xxx: have a test where this nests a couple steps

		// if we might have to dispatch, then include __typename
	}
	return overallP, nil
}

func (e *Executor) plan(typIface graphql.Type, selectionSet *SelectionSet, service string) (*Plan, error) {
	switch typ := typIface.(type) {
	case *graphql.NonNull:
		return e.plan(typ.Type, selectionSet, service)

	case *graphql.List:
		return e.plan(typ.Type, selectionSet, service)

	case *graphql.Object:
		// great
		return e.planObject(typ, selectionSet, service)

	case *graphql.Union:
		// XXX
		return e.planUnion(typ, selectionSet, service)

	default:
		return nil, fmt.Errorf("bad typ %v", typIface)
	}
}

func reversePaths(p *Plan) {
	for i := 0; i < len(p.PathStep)/2; i++ {
		j := len(p.PathStep) - 1 - i
		p.PathStep[i], p.PathStep[j] = p.PathStep[j], p.PathStep[i]
	}
	for _, p := range p.After {
		reversePaths(p)
	}
}

func (e *Executor) Plan(query *graphql.RawSelectionSet) (*Plan, error) {
	allTypes := make(map[graphql.Type]string)
	if err := collectTypes(e.schema.Schema.Query, allTypes); err != nil {
		return nil, err
	}
	reversedTypes := make(map[string]graphql.Type)
	for typ, name := range allTypes {
		reversedTypes[name] = typ
	}

	flattened, err := flatten(convert(query), e.schema.Schema.Query, reversedTypes)
	if err != nil {
		return nil, err
	}

	p, err := e.plan(e.schema.Schema.Query, flattened, "no-such-service")
	if err != nil {
		return nil, err
	}
	reversePaths(p)
	return p, nil
}

func convert(query *graphql.RawSelectionSet) *SelectionSet {
	if query == nil {
		return nil
	}

	var converted []*Selection
	for _, selection := range query.Selections {
		if selection.Alias == "" {
			selection.Alias = selection.Name
		}
		converted = append(converted, &Selection{
			Name:         selection.Name,
			Alias:        selection.Alias,
			Args:         selection.Args,
			SelectionSet: convert(selection.SelectionSet),
		})
	}
	// XXX: janky hack
	var fragments []*Fragment
	for _, fragment := range query.Fragments {
		fragments = append(fragments, &Fragment{
			On:           fragment.On,
			SelectionSet: convert(fragment.SelectionSet),
		})
	}

	return &SelectionSet{
		Selections: converted,
		Fragments:  fragments,
	}
}

// todo
// project. union types
// project. fragments
// __typename
//
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
// XXX: cache queries and plans? even better, cache selection sets downstream?
// XXX: precompile queries and query plans???
//
// xxx: schema migrations? moving fields?
//
// dependency sets
