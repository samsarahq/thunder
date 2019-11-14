package federation

import "github.com/samsarahq/thunder/graphql"

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
