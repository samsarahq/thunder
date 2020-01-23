package federation

import (
	"errors"
	"fmt"
	"sort"

	"github.com/samsarahq/thunder/graphql"
)

type StepKind int

const (
	KindType StepKind = iota
	KindField
)

// PathStep defines where in the plan this part of the query came from
type PathStep struct {
	Kind StepKind // KindType indicates a selection type and KindField indicates a union type
	Name string   // Name of the previous steps this plan is nested on
}

// Plan breaks the query down into subqueries that can be resolved by a single graphql server
type Plan struct {
	Path         []PathStep            // Pathsetp defines what the selections this plan is nested on
	Service      string                // Service that resolves this path step
	Kind         string                // Kind is either a query or mutation
	Type         string                // Type is the name of the object type each subplan is nested on
	SelectionSet *graphql.SelectionSet // Selections that will be resolved in this part of the plan
	After        []*Plan               // Subplans from nested queries on this path
}

type Planner struct {
	schema    *SchemaWithFederationInfo
	flattener *flattener
}

func printPlan(rootPlan *Plan) {
	for _, plan := range rootPlan.After {
		for _, selection := range plan.SelectionSet.Selections {
			fmt.Println("service: ", plan.Service)
			fmt.Println(selection.Name)
			printSelections(selection.SelectionSet)
			fmt.Println("")
		}
		for _, subPlan := range plan.After {
			printPlan(subPlan)
		}
	}
}

func printSelections(selectionSet *graphql.SelectionSet) {
	if selectionSet != nil {
		for _, subSelection := range selectionSet.Selections {
			fmt.Println(" ", subSelection.Name)
			printSelections(subSelection.SelectionSet)
		}
		fmt.Println(" fragments")
		for _, subFragment := range selectionSet.Fragments {
			printSelections(subFragment.SelectionSet)
		}
	}
}

func (e *Planner) planObject(typ *graphql.Object, selectionSet *graphql.SelectionSet, service string) (*Plan, error) {
	p := &Plan{
		Type:         typ.Name,
		Service:      service,
		SelectionSet: &graphql.SelectionSet{},
		After:        nil,
		Kind:         "query",
	}

	var localSelections []*graphql.Selection
	selectionsByService := make(map[string][]*graphql.Selection)

	// Flattened queries should not have any fragments
	if len(selectionSet.Fragments) > 0 {
		return nil, errors.New("selectionSet has fragments, expected flattened query")
	}

	for _, selection := range selectionSet.Selections {
		if selection.Name == "__typename" {
			localSelections = append(localSelections, selection)
			continue
		}

		// Check that the selection name is an expected field
		field, ok := typ.Fields[selection.Name]
		if !ok {
			return nil, fmt.Errorf("typ %s has no field %s", typ.Name, selection.Name)
		}

		fieldInfo := e.schema.Fields[field]

		// Prioritize resolving as many fields as we can in the current service
		if fieldInfo.Services[service] {
			localSelections = append(localSelections, selection)
		} else {
			serviceWithField := ""

			for service, hasField := range fieldInfo.Services {
				if hasField {
					serviceWithField = service
				}
			}

			selectionsByService[serviceWithField] = append(
				selectionsByService[serviceWithField], selection)
		}
	}

	// Create a plan for all the selections that can be resolved in the current graphql service
	for _, selection := range localSelections {
		field := typ.Fields[selection.Name]
		var childPlan *Plan
		if selection.SelectionSet != nil {
			var err error
			childPlan, err = e.plan(field.Type, selection.SelectionSet, service)
			if err != nil {
				return nil, fmt.Errorf("planning for %s: %v", selection.Name, err)
			}
		}

		selectionCopy := &graphql.Selection{
			Alias:        selection.Alias,
			Name:         selection.Name,
			Args:         selection.Args,
			SelectionSet: selection.SelectionSet,
			UnparsedArgs: selection.UnparsedArgs,
			ParentType:   selection.ParentType,
		}
		p.SelectionSet.Selections = append(p.SelectionSet.Selections, selectionCopy)

		if childPlan != nil {
			for _, subPlan := range childPlan.After {
				subPlan.Path = append(subPlan.Path, PathStep{Kind: KindField, Name: selection.Alias})
				p.After = append(p.After, subPlan)
			}
		}
	}

	// needKey is true for selections on other graphql servers
	needKey := false

	// List of services with selections in the query
	var otherServices []string
	for other := range selectionsByService {
		otherServices = append(otherServices, other)
	}
	sort.Strings(otherServices)

	// Create a plan for all selections that can be resolved in other graphql queries
	for _, other := range otherServices {
		selections := selectionsByService[other]
		needKey = true

		subPlan, err := e.plan(typ, &graphql.SelectionSet{Selections: selections}, other)
		if err != nil {
			return nil, fmt.Errorf("planning for %s: %v", other, err)
		}

		p.After = append(p.After, subPlan)
	}

	// If the selection set doesn't have "__federation" key add it to the selection set
	// "__federation" indicates a seperate subplan that will be dispatched to a graphql server
	if needKey {
		hasKey := false
		for _, selection := range p.SelectionSet.Selections {
			if selection.Name == "__federation" && selection.Alias == "__federation" {
				hasKey = true
			} else if selection.Name == "__federation" || selection.Alias == "__federation" {
				return nil, fmt.Errorf("Both the selection name and alias have to be __federation")
			}
		}
		if !hasKey {
			p.SelectionSet.Selections = append(p.SelectionSet.Selections, &graphql.Selection{
				Name:  "__federation",
				Alias: "__federation",
				Args:  map[string]interface{}{},
			})
		}
	}

	return p, nil

}

func (e *Planner) planUnion(typ *graphql.Union, selectionSet *graphql.SelectionSet, service string) (*Plan, error) {
	plan := &Plan{
		// TODO: only include __typename if needed for dispatching? ie. len(types) > 1 and len(fragments) > 0?
		// TODO: ensure __typename doesn't conflict with another field?

		SelectionSet: &graphql.SelectionSet{
			Selections: []*graphql.Selection{
				{
					Name:  "__typename",
					Alias: "__typename",
					Args:  map[string]interface{}{},
				},
			},
		},
		Kind: "query",
	}

	for _, selection := range selectionSet.Selections {
		if selection.Name != "__typename" {
			return nil, fmt.Errorf("unexpected selection %s on union", selection.Name)
		}
		plan.SelectionSet.Selections = append(plan.SelectionSet.Selections, selection)
	}

	// We expect at most one suplan per type since the query is flattened
	seenFragments := make(map[string]struct{})

	for _, fragment := range selectionSet.Fragments {
		// Enforce flattened schema.
		if _, ok := seenFragments[fragment.On]; ok {
			return nil, fmt.Errorf("reused fragment %s, expected flattened query", fragment.On)
		}
		seenFragments[fragment.On] = struct{}{}

		// All fragments must be on concrete types
		typ, ok := typ.Types[fragment.On]
		if !ok {
			return nil, fmt.Errorf("unexpected fragment on %s for typ %s", fragment.On, typ.Name)
		}

		// Create a plan for all fragment types
		concretePlan, err := e.plan(typ, fragment.SelectionSet, service)
		if err != nil {
			return nil, err
		}

		// Query the fields known to the current with a local fragment.
		plan.SelectionSet.Fragments = append(plan.SelectionSet.Fragments, &graphql.Fragment{
			On:           typ.Name,
			SelectionSet: concretePlan.SelectionSet,
		})

		// Make subplans conditional on the current type.
		for _, subPlan := range concretePlan.After {
			subPlan.Path = append(subPlan.Path, PathStep{Kind: KindType, Name: typ.Name})
			plan.After = append(plan.After, subPlan)
		}
	}

	return plan, nil
}

func (e *Planner) plan(typIface graphql.Type, selectionSet *graphql.SelectionSet, service string) (*Plan, error) {
	switch typ := typIface.(type) {
	case *graphql.NonNull:
		return e.plan(typ.Type, selectionSet, service)

	case *graphql.List:
		return e.plan(typ.Type, selectionSet, service)

	case *graphql.Object:
		return e.planObject(typ, selectionSet, service)

	case *graphql.Union:
		return e.planUnion(typ, selectionSet, service)

	default:
		return nil, fmt.Errorf("bad typ %v", typIface)
	}
}

// reversePaths reverses all paths in the plan and its subplans.
//
// Building reverse plans is easier with append, this cleans up the mess.
func reversePaths(p *Plan) {
	for i := 0; i < len(p.Path)/2; i++ {
		j := len(p.Path) - 1 - i
		p.Path[i], p.Path[j] = p.Path[j], p.Path[i]
	}
	for _, p := range p.After {
		reversePaths(p)
	}
}

func (e *Planner) planRoot(query *graphql.Query) (*Plan, error) {
	var schema graphql.Type
	switch query.Kind {
	case "query":
		schema = e.schema.Schema.Query
	case "mutation":
		schema = e.schema.Schema.Mutation
	default:
		return nil, fmt.Errorf("unknown query kind %s", query.Kind)
	}

	flattened, err := e.flattener.flatten(query.SelectionSet, schema)
	if err != nil {
		return nil, err
	}

	p, err := e.plan(schema, flattened, "gateway-coordinator-service")
	if err != nil {
		return nil, err
	}

	if query.Kind == "mutation" {
		if len(p.After) > 1 {
			return nil, errors.New("only support 1 mutation step to maintain ordering")
		}
		for _, p := range p.After {
			p.Kind = "mutation"
		}
	}

	reversePaths(p)

	return p, nil
}
