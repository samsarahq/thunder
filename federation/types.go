package federation

import (
	"encoding/json"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/thunderpb"
)

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

func unmarshalPbSelectionSet(selectionSet *thunderpb.SelectionSet) (*SelectionSet, error) {
	if selectionSet == nil {
		return nil, nil
	}

	selections := make([]*Selection, 0, len(selectionSet.Selections))
	for _, selection := range selectionSet.Selections {
		children, err := unmarshalPbSelectionSet(selection.SelectionSet)
		if err != nil {
			return nil, err
		}

		var args map[string]interface{}
		if len(selection.Arguments) != 0 {
			if err := json.Unmarshal(selection.Arguments, &args); err != nil {
				return nil, err
			}
		}

		selections = append(selections, &Selection{
			Name:         selection.Name,
			Alias:        selection.Alias,
			SelectionSet: children,
			Args:         args,
		})
	}

	fragments := make([]*Fragment, 0, len(selectionSet.Fragments))
	for _, fragment := range selectionSet.Fragments {
		selections, err := unmarshalPbSelectionSet(fragment.SelectionSet)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, &Fragment{
			On:           fragment.On,
			SelectionSet: selections,
		})
	}

	return &SelectionSet{
		Selections: selections,
		Fragments:  fragments,
	}, nil
}

func marshalPbSelections(selectionSet *SelectionSet) (*thunderpb.SelectionSet, error) {
	if selectionSet == nil {
		return nil, nil
	}

	selections := make([]*thunderpb.Selection, 0, len(selectionSet.Selections))
	for _, selection := range selectionSet.Selections {
		children, err := marshalPbSelections(selection.SelectionSet)
		if err != nil {
			return nil, err
		}

		var args []byte
		if selection.Args != nil {
			var err error
			args, err = json.Marshal(selection.Args)
			if err != nil {
				return nil, err
			}
		}

		selections = append(selections, &thunderpb.Selection{
			Name:         selection.Name,
			Alias:        selection.Alias,
			SelectionSet: children,
			Arguments:    args,
		})
	}

	fragments := make([]*thunderpb.Fragment, 0, len(selectionSet.Fragments))
	for _, fragment := range selectionSet.Fragments {
		selections, err := marshalPbSelections(fragment.SelectionSet)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, &thunderpb.Fragment{
			On:           fragment.On,
			SelectionSet: selections,
		})
	}

	return &thunderpb.SelectionSet{
		Selections: selections,
		Fragments:  fragments,
	}, nil
}
