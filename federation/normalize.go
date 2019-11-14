package federation

import (
	"fmt"
	"sort"

	"github.com/samsarahq/thunder/graphql"
)

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
		selections := make([]*Selection, 0, len(selectionSet.Selections))
		// XXX: test that we flatten recursively??
		for _, selection := range selectionSet.Selections {
			if selection.Name == "__typename" {
				if selection.SelectionSet != nil || len(selection.Args) != 0 {
					return nil, fmt.Errorf("typename takes no selection or args")
				}
				selections = append(selections, &Selection{
					Name:         selection.Name,
					Alias:        selection.Alias,
					Args:         map[string]interface{}{},
					SelectionSet: nil,
				})
				continue
			}

			field, ok := typ.Fields[selection.Name]
			if !ok {
				return nil, fmt.Errorf("unknown field %s on typ %s", selection.Name, typ.Name)
			}
			selectionSet, err := flatten(selection.SelectionSet, field.Type, allTypes)
			if err != nil {
				return nil, err
			}
			selections = append(selections, &Selection{
				Name:         selection.Name,
				Alias:        selection.Alias,
				Args:         selection.Args,
				SelectionSet: selectionSet,
			})
		}

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
			plan, err := flatten(selectionSet, obj, allTypes)
			if err != nil {
				return nil, err
			}
			fragments = append(fragments, &Fragment{
				On:           obj.Name,
				SelectionSet: plan,
			})
		}
		sort.Slice(fragments, func(a, b int) bool {
			return fragments[a].On < fragments[b].On
		})

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
