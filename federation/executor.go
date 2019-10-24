package main

import (
	"context"

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
	Service string
	Args    map[string]TypeName
	Type    TypeName
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

type InputObject struct {
	// xxx; how do we demarcate optional?
	Fields map[string]TypeName
}

type Object struct {
	Fields map[string]Field
}

type Selection struct {
	Alias      string
	Name       string
	Args       map[string]interface{}
	Selections []*Selection
}

type SubPlan struct {
	Path []string
	*Plan
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

func (e *Executor) runOnService(service string, keys []interface{}, selections []*Selection) []interface{} {
	schema := e.Executors[service]

	// xxx: detect if root (?)

	var selectionSet *graphql.RawSelectionSet

	if keys == nil {
		// Root query
		selectionSet = convertSelectionSet(selections)
	} else {
		selectionSet = &graphql.RawSelectionSet{
			Selections: []*graphql.RawSelection{
				{
					Name:  "foosFromFederationKeys",
					Alias: "results",
					Args: map[string]interface{}{
						"keys": keys,
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
	Service    string
	Selections []*Selection
	After      []SubPlan
}

// XXX: have a plan about failed conversions and nils everywhere.

func (e *Executor) execute(p *Plan, keys []interface{}) []interface{} {
	res := e.runOnService(p.Service, keys, p.Selections)

	for _, subPlan := range p.After {
		// go func() {
		var targets []interface{}
		targets = []interface{}{res}

		// xxx; reverse path
		for _, elem := range subPlan.Path {
			var newTargets []interface{}

			// spew.Dump(elem, targets)

			// XXX: have a clearer plan about this
			// XXX: can save allocations if we DFS instead of BFS
			if len(targets) > 0 {
				if _, ok := targets[0].([]interface{}); ok {
					for _, k := range targets {
						for _, j := range k.([]interface{}) {
							newTargets = append(newTargets, j)
						}
					}
					targets = newTargets
					newTargets = []interface{}{}
				}
			}

			for _, k := range targets {
				newTargets = append(newTargets, k.(map[string]interface{})[elem])
			}
			targets = newTargets
		}

		if len(targets) > 0 {
			if _, ok := targets[0].([]interface{}); ok {
				var newTargets []interface{}
				for _, k := range targets {
					for _, j := range k.([]interface{}) {
						newTargets = append(newTargets, j)
					}
				}
				targets = newTargets
			}
		}

		var keys []interface{}
		for _, target := range targets {
			keys = append(keys, target.(map[string]interface{})["federationKey"])
		}

		spew.Dump(targets, keys)

		// XXX: don't execute here yet??? i mean we can but why? simpler?????? could go back to root?

		results := e.execute(subPlan.Plan, keys)

		for i := range targets {
			for k, v := range results[i].(map[string]interface{}) {
				targets[i].(map[string]interface{})[k] = v
			}
		}
		// }()
	}

	return res
}

func (e *Executor) plan(typ *Object, selections []*Selection, service string) *Plan {
	p := &Plan{
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

		selectionsByService[field.Service] = append(
			selectionsByService[field.Service], selection)
	}

	spew.Dump(service, selectionsByService)

	// if we encounter a fragment, we find a branch
	// either we hit it, or we don't. must make plan for both cases?
	//
	// very snazzily we could merge subplans.

	for _, selection := range selectionsByService[service] {
		field := typ.Fields[selection.Name]

		var childPlan *Plan
		if selection.Selections != nil {
			childPlan = e.plan(e.Types[field.Type], selection.Selections, service)
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
				p.After = append(p.After, SubPlan{
					Path: append(subPlan.Path, selection.Alias),
					Plan: subPlan.Plan,
				})
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
		subPlan := e.plan(typ, selections, other)

		p.After = append(p.After, SubPlan{
			Path: []string{},
			Plan: subPlan,
		})
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
			})
		}
	}

	return p
}

func (e *Executor) Plan(typ *Object, selections []*Selection) *Plan {
	return e.plan(typ, selections, "no-such-service")
}

func testfoo() {
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
			},
		},
	}

	executors := map[string]*graphql.Schema{
		"schema1": schema1().MustBuild(),
		"schema2": schema2().MustBuild(),
	}

	// executors["schema1"]

	e := &Executor{
		Types:     types,
		Executors: executors,
	}

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
			},
		},
	}

	plan := e.Plan(e.Types["Query"], query).After

	// XXX: have to deal with multiple plans here
	res := e.execute(plan[0].Plan, nil)

	spew.Dump(res)
}

// todo
// project. end to end test
// in process
// make code actually work
// nail down some unit tests
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
// project. union and fragment types
//
// project. schema (un)marshaling
// do that

// XXX: cache queries and plans? even better, cache selection sets downstream?
