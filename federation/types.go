package federation

import (
	"encoding/json"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/thunderpb"
)

func unmarshalPbSelectionSet(selectionSet *thunderpb.SelectionSet) (*graphql.RawSelectionSet, error) {
	if selectionSet == nil {
		return nil, nil
	}

	selections := make([]*graphql.RawSelection, 0, len(selectionSet.Selections))
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

		selections = append(selections, &graphql.RawSelection{
			Name:         selection.Name,
			Alias:        selection.Alias,
			SelectionSet: children,
			Args:         args,
		})
	}

	fragments := make([]*graphql.RawFragment, 0, len(selectionSet.Fragments))
	for _, fragment := range selectionSet.Fragments {
		selections, err := unmarshalPbSelectionSet(fragment.SelectionSet)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, &graphql.RawFragment{
			On:           fragment.On,
			SelectionSet: selections,
		})
	}

	return &graphql.RawSelectionSet{
		Selections: selections,
		Fragments:  fragments,
	}, nil
}

func marshalPbSelections(selectionSet *graphql.RawSelectionSet) (*thunderpb.SelectionSet, error) {
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
