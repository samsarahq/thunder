package federation

import (
	"fmt"
	"sort"

	"github.com/samsarahq/thunder/graphql"
)

func (e *Executor) planObject(typ *graphql.Object, selectionSet *SelectionSet, service string) (*Plan, error) {
	// needTypename := true

	p := &Plan{
		Type:         typ.Name,
		Service:      service,
		SelectionSet: &SelectionSet{},
		After:        nil,
	}

	// XXX: pass in sub-path (and sub-plan slice?) to make sub-plan munging simpler?

	// - should we type check here?
	// - what do we here with fragments? inline them?
	// - the executor might need to know the types to switch?

	// - refactor to return array of subplans with preferential treatment for given service?
	// eh whatever.

	var localSelections []*Selection
	selectionsByService := make(map[string][]*Selection)

	// A flattened selection set on an object will have only selectoins, no fragments.
	// XXX: assert?

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

	// spew.Dump(service, selectionsByService)

	// if we encounter a fragment, we find a branch
	// either we hit it, or we don't. must make plan for both cases?
	//
	// very snazzily we could merge subplans.

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
