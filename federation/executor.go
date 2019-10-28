package main

import (
	"context"
	"encoding/json"

	"github.com/davecgh/go-spew/spew"
	"github.com/samsarahq/thunder/graphql"
)

type Executor struct {
	Executors map[string]*graphql.Schema

	Types map[TypeName]*Object
}

type TypeName string

type Type interface{}

// oooh.. maybe we need to support introspection at the aggregator level... :)

type Scalar struct {
	Name TypeName
}

type Union struct {
	Types []TypeName
}

type Field struct {
	// XXX: services?
	Service  string
	Services map[string]bool
	Args     map[string]TypeName
	Type     TypeName
}

/*
type TypeRef struct {
	TypeName string
	NonNull  *NonNull
	List     *List
}

type NonNull struct {
}

type List struct {
}

type Modifier string

const (
	NonNull Modifier = "NonNull"
	List    Modifier = "List"
)

type ModifiedType struct {
	Type      TypeName
	Modifiers []Modifier
}
*/

type InputObject struct {
	// xxx; how do we demarcate optional?
	Fields map[string]TypeName
}

type Object struct {
	Fields map[string]*Field
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

func (e *Executor) runOnService(service string, typName string, keys []interface{}, selections []*Selection) []interface{} {
	schema := e.Executors[service]

	// xxx: detect if root (?)

	var selectionSet *graphql.RawSelectionSet

	if keys == nil {
		// Root query
		selectionSet = convertSelectionSet(selections)
	} else {

		var garbage interface{}
		bytes, err := json.Marshal(keys)
		if err != nil {
			panic(err)
		}
		if err := json.Unmarshal(bytes, &garbage); err != nil {
			panic(err)
		}

		// XXX: halp
		selectionSet = &graphql.RawSelectionSet{
			Selections: []*graphql.RawSelection{
				{
					Name:  typName + "sFromFederationKeys",
					Alias: "results",
					Args: map[string]interface{}{
						"keys": garbage, // keys,
					},
					SelectionSet: convertSelectionSet(selections),
				},
			},
		}
	}

	gqlExec := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	res, err := gqlExec.Execute(context.Background(), schema.Query, nil, &graphql.Query{
		Kind:         "query",
		Name:         "",
		SelectionSet: selectionSet,
	})
	if err != nil {
		panic(err)
	}

	if keys == nil {
		return []interface{}{res}
	} else {
		return res.(map[string]interface{})["results"].([]interface{})
	}
}

type Plan struct {
	Path       []string
	Service    string
	Type       string
	Selections []*Selection
	After      []*Plan
}

// XXX: have a plan about failed conversions and nils everywhere.

func (e *Executor) execute(p *Plan, keys []interface{}) []interface{} {
	res := e.runOnService(p.Service, p.Type, keys, p.Selections)

	for _, subPlan := range p.After {
		var targets []interface{}
		// targets = []interface{}{res}

		// DFS to follow path

		var search func(node interface{}, path []string)
		search = func(node interface{}, path []string) {
			// XXX: encode list flattening in path?
			if slice, ok := node.([]interface{}); ok {
				for _, elem := range slice {
					search(elem, path)
				}
				return
			}

			if len(path) == 0 {
				targets = append(targets, node)
				return
			}

			search(node.(map[string]interface{})[path[0]], path[1:])
		}

		// xxx; reverse path
		search(res, subPlan.Path)

		var keys []interface{}
		for _, target := range targets {
			keys = append(keys, target.(map[string]interface{})["federationKey"])
		}

		// spew.Dump(targets, keys)

		// XXX: don't execute here yet??? i mean we can but why? simpler?????? could go back to root?

		// XXX: go
		results := e.execute(subPlan, keys)

		for i := range targets {
			for k, v := range results[i].(map[string]interface{}) {
				targets[i].(map[string]interface{})[k] = v
			}
		}
	}

	return res
}

func (e *Executor) plan(typName string, typ *Object, selections []*Selection, service string) *Plan {
	p := &Plan{
		Type:       typName,
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
		field := typ.Fields[selection.Name]

		// if we can stick to the current service, stay there
		if field.Services[service] {
			selectionsByService[service] = append(
				selectionsByService[service], selection)
		} else {
			selectionsByService[field.Service] = append(
				selectionsByService[field.Service], selection)
		}
	}

	// spew.Dump(service, selectionsByService)

	// if we encounter a fragment, we find a branch
	// either we hit it, or we don't. must make plan for both cases?
	//
	// very snazzily we could merge subplans.

	for _, selection := range selectionsByService[service] {
		field := typ.Fields[selection.Name]

		var childPlan *Plan
		if selection.Selections != nil {
			childPlan = e.plan(string(field.Type), e.Types[field.Type], selection.Selections, service)
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
		subPlan := e.plan(typName, typ, selections, other)

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

	return p
}

func (e *Executor) Plan(typ *Object, selections []*Selection) *Plan {
	return e.plan("", typ, selections, "no-such-service")
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

func main() {
	executors := map[string]*graphql.Schema{
		"schema1": schema1().MustBuild(),
		"schema2": schema2().MustBuild(),
	}

	types := convertSchema(executors)

	// executors["schema1"]

	e := &Executor{
		Types:     types,
		Executors: executors,
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

	plan := e.Plan(e.Types["Query"], query).After

	// XXX: have to deal with multiple plans here
	res := e.execute(plan[0], nil)

	spew.Dump(res)
}

// todo
// project. end to end test
// nail down some unit tests
// kill panics, return errors
//
// project. end to end with rpcs.
// rpc for invocation
//
// project. schema api
// design it
// implement it
//
// project. harden APIs
//
// project. fragments
//
// project. union types
//
// project. schema (un)marshaling
// do that

// XXX: cache queries and plans? even better, cache selection sets downstream?

/*
	types := map[TypeName]*Object{
		"Query": {
			Fields: map[string]Field{
				"f": {
					Service: "schema1",
					Args:    nil,
					Type:    "foo",
				},
				"fff": {
					Service: "schema1",
					Args:    nil,
					Type:    "foo",
				},
				// XXX: federate other directon as well!
				// XXX: federate multiple types?
				"foosFromFederationKeys": {
					Service: "schema2",
					Args:    nil, // XXX
					Type:    "foo",
				},
				"barsFromFederationKeys": {
					Service: "schema1",
					Args:    nil, // XXX
					Type:    "bar",
				},
			},
		},
		"foo": {
			Fields: map[string]Field{
				"federationKey": {
					Service: "schema1",
					Args:    nil,
					Type:    "string",
				},
				"hmm": {
					Service: "schema1",
					Args:    nil,
					Type:    "string",
				},
				"ok": {
					Service: "schema2",
					Args:    nil,
					Type:    "string",
				},
				"bar": {
					Service: "schema2",
					Type:    "bar",
				},
			},
		},
		"bar": {
			Fields: map[string]Field{
				"id": {
					Service: "schema2",
					Type:    "int64",
				},
				"federationKey": {
					Service: "schema2",
					Args:    nil,
					Type:    "int64",
				},
				"baz": {
					Service: "schema1",
					Args:    nil,
					Type:    "string",
				},
			},
		},
	}
*/

/*
	query := []*Selection{
		{
			Name:  "fff",
			Alias: "fff",
			Selections: []*Selection{
				{
					Name:  "hmm",
					Alias: "hmm",
				},
				{
					Name:  "ok",
					Alias: "ok",
				},
				{
					Name:  "bar",
					Alias: "bar",
					Selections: []*Selection{
						{
							Name:  "id",
							Alias: "id",
						},
						{
							Name:  "baz",
							Alias: "baz",
						},
					},
				},
			},
		},
	}
*/
