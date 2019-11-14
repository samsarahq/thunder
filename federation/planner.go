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

func (e *Executor) planObject(typ *graphql.Object, selectionSet *SelectionSet, service string) (*Plan, error) {
	p := &Plan{
		Type:         typ.Name,
		Service:      service,
		SelectionSet: &SelectionSet{},
		After:        nil,
	}

	// XXX: pass in sub-path (and sub-plan slice?) to make sub-plan munging simpler?
	// - should we type check here?

	// - refactor to return array of subplans with preferential treatment for given service?
	// eh whatever.

	var localSelections []*Selection
	selectionsByService := make(map[string][]*Selection)

	// Assert that we are working with a flattened query.
	if len(selectionSet.Fragments) > 0 {
		return nil, errors.New("selectionSet has fragments, expected flattened query")
	}

	for _, selection := range selectionSet.Selections {
		if selection.Name == "__typename" {
			localSelections = append(localSelections, selection)
			continue
		}

		field, ok := typ.Fields[selection.Name]
		if !ok {
			return nil, fmt.Errorf("typ %s has no field %s", typ.Name, selection.Name)
		}

		fieldInfo := e.schema.Fields[field]

		// if we can stick to the current service, stay there
		if fieldInfo.Services[service] {
			localSelections = append(localSelections, selection)
		} else {
			selectionsByService[fieldInfo.Service] = append(
				selectionsByService[fieldInfo.Service], selection)
		}
	}

	for _, selection := range localSelections {
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
		otherServices = append(otherServices, other)
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
	plan := &Plan{
		// XXX: only include __typename if needed for dispatching? ie. len(types) > 1 and len(fragments) > 0?
		// XXX: ensure __typename doesn't conflict with another field?
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

	for _, selection := range selectionSet.Selections {
		if selection.Name != "__typename" {
			return nil, fmt.Errorf("unexpected selection %s on union", selection.Name)
		}
		plan.SelectionSet.Selections = append(plan.SelectionSet.Selections, selection)
	}

	// Expect a flattened query, so we see each type at most once and create at
	// most one set of sub-plans per type.
	seenFragments := make(map[string]struct{})

	for _, fragment := range selectionSet.Fragments {
		// Enforce flattened schema.
		if _, ok := seenFragments[fragment.On]; ok {
			return nil, fmt.Errorf("reused fragment %s, expected flattened query", fragment.On)
		}
		seenFragments[fragment.On] = struct{}{}

		// This is a flattened selection set, so all fragments must be on a concrete type.
		typ, ok := typ.Types[fragment.On]
		if !ok {
			return nil, fmt.Errorf("unexpected fragment on %s for typ %s", fragment.On, typ.Name)
		}

		// Plan for the concrete object type.
		concretePlan, err := e.plan(typ, fragment.SelectionSet, service)
		if err != nil {
			return nil, err
		}

		// Query the fields known to the current with a local fragment.
		plan.SelectionSet.Fragments = append(plan.SelectionSet.Fragments, &Fragment{
			On:           typ.Name,
			SelectionSet: concretePlan.SelectionSet,
		})

		// Make subplans conditional on the current type.
		for _, subPlan := range concretePlan.After {
			subPlan.PathStep = append(subPlan.PathStep, PathStep{Kind: KindType, Name: typ.Name})
			plan.After = append(plan.After, subPlan)
		}
	}

	return plan, nil
}

func (e *Executor) plan(typIface graphql.Type, selectionSet *SelectionSet, service string) (*Plan, error) {
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
	for i := 0; i < len(p.PathStep)/2; i++ {
		j := len(p.PathStep) - 1 - i
		p.PathStep[i], p.PathStep[j] = p.PathStep[j], p.PathStep[i]
	}
	for _, p := range p.After {
		reversePaths(p)
	}
}

func (e *Executor) Plan(query *graphql.RawSelectionSet) (*Plan, error) {
	flattened, err := e.flatten(convert(query), e.schema.Schema.Query)
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
